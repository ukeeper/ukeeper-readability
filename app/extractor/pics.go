package extractor

import (
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func (f UReadability) extractPics(iselect *goquery.Selection, url string) (mainImage string, allImages []string, ok bool) {

	images := make(map[int]string)

	iselect.Each(func(i int, s *goquery.Selection) {
		if im, ok := s.Attr("src"); ok {
			images[f.getImageSize(im)] = im
			allImages = append(allImages, im)
		}
	})

	if len(images) == 0 {
		return "", nil, false
	}

	//get biggest picture
	keys := make([]int, 0, len(images))
	for k := range images {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(keys)))
	mainImage = images[keys[0]]
	if f.Debug {
		log.Printf("total images from %s = %d, main=%s (%d)", url, len(images), mainImage, keys[0])
	}
	return mainImage, allImages, true
}

func (f UReadability) getImageSize(url string) int {
	httpClient := &http.Client{Timeout: time.Second * 30}
	req, err := http.NewRequest("GET", url, nil)
	req.Close = true
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("can't get %s, error=%v", url, err)
		return 0
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("failed to get %s, err=%v", url, err)
		return 0
	}
	return len(data)
}
