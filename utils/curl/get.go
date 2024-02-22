package curl

import (
	"encoding/json"
	"fmt"
	"github.com/TskFok/DockerImgSync/app/global"
	"net/http"
	"net/url"
)

func Get(host string, responseBody any, header http.Header) int {
	client := &http.Client{}

	if global.ProxyHost != "" {
		proxyUrl, err := url.Parse(global.ProxyHost)
		if err != nil {
			panic(err)
		}

		transport := &http.Transport{
			Proxy: http.ProxyURL(proxyUrl),
		}

		client = &http.Client{
			Transport: transport,
		}
	}

	res, err := http.NewRequest("GET", host, nil)

	if err != nil {
		fmt.Println("error")
	}

	res.Header = header

	rep, _ := client.Do(res)

	if rep.StatusCode == http.StatusOK {
		decode := json.NewDecoder(rep.Body)

		decodeErr := decode.Decode(responseBody)

		if decodeErr != nil {
			fmt.Println(decodeErr.Error())
		}
	}
	resErr := rep.Body.Close()

	if resErr != nil {
		fmt.Println(resErr.Error())
	}

	return rep.StatusCode
}
