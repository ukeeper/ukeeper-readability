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
	imageFetchTimeout       = 30 * time.Second
	maxImageBytes           = 10 << 20 // cap when measuring an image, avoids buffering huge files
	maxConcurrentImageFetch = 8        // limit parallel image probes per page
)

// imageClient returns a lazily-built HTTP client for image size probes, sharing one client across
// all fetches and honoring BlockPrivateNetworks to guard against SSRF via image URLs.
func (f *UReadability) imageClient() *http.Client {
	f.imgClientOnce.Do(func() {
		f.imgClient = &http.Client{Timeout: imageFetchTimeout}
		if f.BlockPrivateNetworks {
			f.imgClient.Transport = safeTransport(imageFetchTimeout)
		}
	})
	return f.imgClient
}

func (f *UReadability) extractPics(ctx context.Context, iselect *goquery.Selection, url string) (mainImage string, allImages []string, ok bool) {
	images := make(map[int]string)

	type imgInfo struct {
		url  string
		size int
	}
	resCh := make(chan imgInfo)
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentImageFetch)

	iselect.Each(func(_ int, s *goquery.Selection) {
		if im, exists := s.Attr("src"); exists {
			wg.Go(func() {
				sem <- struct{}{}
				defer func() { <-sem }()
				resCh <- imgInfo{url: im, size: f.getImageSize(ctx, im)}
			})
		}
	})

	go func() {
		wg.Wait()
		close(resCh)
	}()

	for r := range resCh {
		images[r.size] = r.url
		allImages = append(allImages, r.url)
	}
	sort.Strings(allImages)
	if len(images) == 0 {
		return "", nil, false
	}

	// get the biggest picture
	keys := make([]int, 0, len(images))
	for k := range images {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(keys)))
	mainImage = images[keys[0]]
	log.Printf("[DEBUG] total images from %s = %d, main=%s (%d)", url, len(images), mainImage, keys[0])
	return mainImage, allImages, true
}

// getImageSize streams the image to measure its byte size without buffering it whole, honoring the
// caller's context and capping the read at maxImageBytes.
func (f *UReadability) getImageSize(ctx context.Context, url string) (size int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		log.Printf("[WARN] can't create request to get pic from %s", url)
		return 0
	}
	req.Close = true
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

	n, err := io.Copy(io.Discard, io.LimitReader(resp.Body, maxImageBytes))
	if err != nil {
		log.Printf("[WARN] failed to get %s, err=%v", url, err)
		return 0
	}
	return int(n)
}
