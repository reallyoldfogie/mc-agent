//go:build none

package models

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

type AssetIndexFileContents struct {
	Objects map[string]Object `json:"objects,omitempty"`
}

type Object struct {
	Hash string `json:"hash,omitempty"`
	Size int64  `json:"size,omitempty"`
}

func (aifc *AssetIndexFileContents) Load(fileName string) error {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}

	err = json.NewDecoder(file).Decode(aifc)
	if err != nil {
		return err
	}
	return nil
}

func (aifc *AssetIndexFileContents) DownloadAssets(outputDir string, excludeFiles map[string]bool) map[string]string {
	downloadedAssets := map[string]string{}

	mu := sync.Mutex{}
	tasks := make(chan string)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for key := range tasks {
				v := aifc.Objects[key]
				fileName := downloadResource(outputDir, key, v.Hash)
				mu.Lock()
				downloadedAssets[key] = fileName
				mu.Unlock()
			}
		}()
	}

	for key := range aifc.Objects {
		skip := false
		for exclude := range excludeFiles {
			if strings.HasPrefix(key, exclude) {
				skip = true
				break
			}
		}

		if skip {
			continue
		}

		tasks <- key
	}

	close(tasks)
	wg.Wait()

	return downloadedAssets
}

func downloadResource(outputDir string, name, hash string) string {
	// download language
	LangURL := "https://resources.download.minecraft.net/" + hash[:2] + "/" + hash
	// fmt.Println(name, ":", LangURL)
	resp, err := http.Get(LangURL)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	// readLang(name, resp.Body)

	pathElements := append([]string{outputDir}, strings.Split(name, "/")...)
	os.MkdirAll(strings.Join(pathElements[:len(pathElements)-1], string(os.PathSeparator)), 0750)
	path := strings.Join(pathElements, string(os.PathSeparator))

	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	io.Copy(file, resp.Body)
	return path
}
