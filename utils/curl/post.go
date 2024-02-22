package curl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func Post(url string, body interface{}, header http.Header, responseBody any) int {
	client := &http.Client{}

	b, e := json.Marshal(body)

	if e != nil {
		fmt.Println("err")
	}

	bReader := bytes.NewReader(b)

	res, err := http.NewRequest("POST", url, bReader)

	if err != nil {
		fmt.Println("error")
	}

	res.Header = header

	rep, err := client.Do(res)

	if err != nil {
		fmt.Println(err.Error())

		return 500
	}

	if rep.StatusCode == http.StatusOK || rep.StatusCode == http.StatusCreated {
		defer rep.Body.Close()
		decode := json.NewDecoder(rep.Body)
		decodeErr := decode.Decode(responseBody)

		if decodeErr != nil {
			fmt.Println(decodeErr.Error())
		}
	}

	return rep.StatusCode
}

func PostAll(url string, body interface{}, header http.Header) []byte {
	client := &http.Client{}

	b, e := json.Marshal(body)

	if e != nil {
		fmt.Println("err")
	}

	bReader := bytes.NewReader(b)

	res, err := http.NewRequest("POST", url, bReader)

	if err != nil {
		fmt.Println("error")
	}

	res.Header = header

	rep, _ := client.Do(res)

	if rep.StatusCode == http.StatusOK {
		defer rep.Body.Close()

		resBt, err := io.ReadAll(rep.Body)
		if err != nil {
			fmt.Println(err.Error())
		}
		return resBt

	}
	return []byte{}
}
