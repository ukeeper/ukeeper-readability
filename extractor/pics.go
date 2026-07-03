package extractor

import (
	"context"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	log "github.com/go-pkgz/lgr"
)

const (
	imageFetchTimeout       = 15 * time.Second // per-image probe timeout, kept below imageProbeBudget
	imageProbeBudget        = 30 * time.Second // overall budget for probing every image on a page
	maxImageBytes           = 10 << 20         // cap when streaming to measure, avoids buffering huge files
	maxConcurrentImageFetch = 8                // limit parallel image probes per page
)

// imageClient returns a lazily-built HTTP client for image size probes, shared across all fetches
// instead of building a fresh client per image.
func (f *UReadability) imageClient() *http.Client {
	f.imgClientOnce.Do(func() {
		f.imgClient = &http.Client{Timeout: imageFetchTimeout}
	})
	return f.imgClient
}

type imgInfo struct {
	url  string
	size int
}

func (f *UReadability) extractPics(ctx context.Context, iselect *goquery.Selection, url string) (mainImage string, allImages []string, ok bool) {
	// bound total image probing so a page full of slow image URLs can't hold the handler open for
	// ceil(n/maxConcurrentImageFetch) * imageFetchTimeout and blow past the server write timeout.
	ctx, cancel := context.WithTimeout(ctx, imageProbeBudget)
	defer cancel()

	resCh := make(chan imgInfo)
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentImageFetch)

	iselect.Each(func(_ int, s *goquery.Selection) {
		if im, exists := s.Attr("src"); exists {
			wg.Go(func() {
				// acquire a slot, but give up if the overall budget is already spent
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					resCh <- imgInfo{url: im, size: 0}
					return
				}
				resCh <- imgInfo{url: im, size: f.getImageSize(ctx, im)}
			})
		}
	})

	go func() {
		wg.Wait()
		close(resCh)
	}()

	var results []imgInfo
	for r := range resCh {
		results = append(results, r)
		allImages = append(allImages, r.url)
	}
	sort.Strings(allImages)
	if len(results) == 0 {
		return "", nil, false
	}

	// pick the largest image; break ties by URL so lead-image selection is deterministic rather
	// than dependent on goroutine scheduling / map iteration order.
	best := results[0]
	for _, r := range results[1:] {
		if r.size > best.size || (r.size == best.size && r.url < best.url) {
			best = r
		}
	}
	log.Printf("[DEBUG] total images from %s = %d, main=%s (%d)", url, len(results), best.url, best.size)
	return best.url, allImages, true
}

// getImageSize measures an image's byte size by streaming it to io.Discard behind a maxImageBytes
// limit, without buffering it whole and honoring the caller's context. The body is actually read
// (rather than trusting Content-Length) so a broken endpoint or a lying header can't inflate an
// image's rank; an unreadable body sizes as 0, matching the previous behavior.
func (f *UReadability) getImageSize(ctx context.Context, url string) (size int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		log.Printf("[WARN] can't create request to get pic from %s", url)
		return 0
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := f.imageClient().Do(req)
	if err != nil {
		log.Printf("[WARN] can't get %s, error=%v", url, err)
		return 0
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			size = 0
		}
	}()

	// treat non-2xx as size 0 so an error page (404/500 HTML) can't be ranked as the lead image
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		log.Printf("[DEBUG] non-2xx (%d) for image %s, sizing as 0", resp.StatusCode, url)
		return 0
	}

	n, err := io.Copy(io.Discard, io.LimitReader(resp.Body, maxImageBytes))
	if err != nil {
		log.Printf("[WARN] failed to get %s, err=%v", url, err)
		return 0
	}
	return int(n)
}
