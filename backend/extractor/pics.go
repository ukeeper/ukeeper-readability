package extractor

import (
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	log "github.com/go-pkgz/lgr"
)

func (f UReadability) extractPics(iselect *goquery.Selection, url string) (mainImage string, allImages []string, ok bool) {
	images := make(map[int]string)

	type imgInfo struct {
		url  string
		size int
	}
	var resCh = make(chan imgInfo)
	var wg sync.WaitGroup

	iselect.Each(func(i int, s *goquery.Selection) {
		if im, ok := s.Attr("src"); ok {
			wg.Add(1)
			go func(url string) {
				size := f.getImageSize(url)
				resCh <- imgInfo{url: url, size: size}
				wg.Done()
			}(im)
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

// getImageSize loads image to get size
func (f UReadability) getImageSize(url string) (size int) {
	httpClient := &http.Client{Timeout: time.Second * 30}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("[WARN] can't create request to get pic from %s", url)
		return 0
	}
	req.Close = true
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[WARN] can't get %s, error=%v", url, err)
		return 0
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			size = 0
		}
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[WARN] failed to get %s, err=%v", url, err)
		return 0
	}
	return len(data)
}
