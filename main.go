package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/natefinch/lumberjack"
)

const RATE_LIMIT = 600

type Status struct {
	URL  string `json:"url"`
	Type string `json:"content_type"`
	Code int    `json:"http_code"`
}

type Response struct {
	Content string `json:"contents"`
	Status  Status `json:"status"`
}

var client *http.Client
var logger *log.Logger

var limiter sync.Map

func init() {
	client = &http.Client{}

	reader := io.MultiWriter(os.Stderr, &lumberjack.Logger{
		Filename: "./main.log",
		MaxSize:  250,
		Compress: true,
	})

	logger = log.New(reader, "", log.Ldate|log.Ltime|log.Lshortfile)
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Allow", "*")
		writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, PUT, DELETE, OPTIONS")
		writer.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Referer, User-Agent")

		next.ServeHTTP(writer, request)
	})
}

func tunnel(URL string, before *http.Request) Response {
	logger.Println("request", URL, before.Header)

	request, err := http.NewRequest("GET", URL, nil)

	if err != nil {
		logger.Println(URL, err)

		return Response{
			Status: Status{
				URL:  URL,
				Code: 500,
			},
		}
	}

	response, err := client.Do(request)

	if err != nil {
		logger.Println(URL, err)

		return Response{
			Status: Status{
				URL:  URL,
				Code: 500,
			},
		}
	}

	plain, err := ioutil.ReadAll(response.Body)

	if err != nil {
		logger.Println(URL, err)

		return Response{
			Status: Status{
				URL:  URL,
				Code: 500,
			},
		}
	}

	result := Response{
		Content: string(plain),
		Status: Status{
			URL:  URL,
			Type: response.Header.Get("Content-Type"),
			Code: response.StatusCode,
		},
	}

	logger.Printf("response %+v\n", result.Status)

	return result
}

func check(address string) bool {
	var value int

	count, _ := limiter.LoadOrStore(address, &value)

	*count.(*int) += 1

	if value > RATE_LIMIT {
		logger.Println(address, "was rate limited")

		return false
	}

	return true
}

func get(writer http.ResponseWriter, request *http.Request) {
	URL := request.URL.Query().Get("url")

	if URL == "" {
		writer.Write([]byte("URL parameter is required."))
		return
	}

	callback := request.URL.Query().Get("callback")

	allowed := check(request.Header.Get("Fly-Client-Ip"))

	if allowed == false {
		writer.Write([]byte(fmt.Sprintf("rate limited: you have a max of %d request per second", RATE_LIMIT)))
		return
	}

	body, _ := json.Marshal(tunnel(URL, request))

	if callback != "" {
		writer.Header().Set("Content-Type", "application/x-javascript")
		body = []byte(callback + "(" + string(body) + ")")
	} else {
		writer.Header().Set("Content-Type", "application/json")
	}

	writer.Write(body)
}

func main() {
	go func() {
		for {
			time.Sleep(1 * time.Minute)

			limiter = sync.Map{}
		}
	}()

	http.Handle("/get", CORS(http.HandlerFunc(get)))
	http.Handle("/", http.FileServer(http.Dir("./static")))

	http.ListenAndServe(":8080", nil)
}
