package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
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
var nonce = int64(0)
var dev bool

func init() {
	client = &http.Client{}

	flag.BoolVar(&dev, "dev", false, "dev mode")
	flag.Parse()

	path := "/logs/log"

	if dev == true {
		path = "./log"
	}

	logger = log.New(&lumberjack.Logger{
		Filename: path,
		MaxSize:  5,
		Compress: true,
	}, "", log.Ldate|log.Ltime|log.Lshortfile)
}

func FormatRequest(request *http.Request) string {
	return fmt.Sprintf("[%v %v %v %v]", request.Header.Get("Fly-Client-Ip"), request.Header, request.Host, request.URL)
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

func view(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		logger.Println("view", FormatRequest(request))

		next.ServeHTTP(writer, request)
	})
}

func tunnel(URL string, before *http.Request) Response {
	nonce += 1

	id := nonce

	logger.Println("request", id, URL, FormatRequest(before))

	request, err := http.NewRequest("GET", URL, nil)

	if err != nil {
		logger.Println("invalid request", id, err)

		return Response{
			Status: Status{
				URL:  URL,
				Code: 500,
			},
		}
	}

	response, err := client.Do(request)

	if err != nil {
		logger.Println("request failed", id, err)

		return Response{
			Status: Status{
				URL:  URL,
				Code: 500,
			},
		}
	}

	plain, err := ioutil.ReadAll(response.Body)

	if err != nil {
		logger.Println("failed to read body", id, err)

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

	logger.Printf("response %d %+v\n", id, result.Status)

	return result
}

func check(address string) bool {
	var value int

	count, _ := limiter.LoadOrStore(address, &value)

	*count.(*int) += 1

	return *count.(*int) < RATE_LIMIT
}

func get(writer http.ResponseWriter, request *http.Request) {
	URL := request.URL.Query().Get("url")

	if URL == "" {
		logger.Println("failed to find parameter", FormatRequest(request))

		writer.Write([]byte("URL parameter is required."))
		return
	}

	callback := request.URL.Query().Get("callback")

	allowed := check(request.Header.Get("Fly-Client-Ip"))

	if allowed == false {
		logger.Println("rate limited", FormatRequest)

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
	http.Handle("/", view(http.FileServer(http.Dir("./static"))))

	panic(http.ListenAndServe(":8080", nil))
}
