package main

import (
	"io"
	"fmt"
	"log"
	"flag"
	"time"
	"bytes"
	"errors"
	"net/url"
	"net/http"
	"encoding/json"
	"golang.org/x/net/websocket"
)

type jsonMap map[string]interface{}
type identifier string
type channel struct {
	id			string
	name   		identifier
	creator	 	identifier
	created 	time.Time
	is_channel	bool
	is_archived bool
	is_general	bool
	has_pins 	bool
	is_member 	bool
}

type baseAnswer struct {
	ok		bool
	error	string
}

type authAnswer struct {
	baseAnswer
	latest_event_ts	time.Time
	channels		[]channel
	ims				string
	cache_version	string
	bots			string
	team			string
	self			string
	groups			string
	users			string
	cache_ts		time.Time
	url 			string
}

const SLACK_API = "https://slack.com/api/%v"
const DUMMY_API_KEY = "xxxx-0123456789-AaAA0AaaaAAa0aAAAaaA0aAa"

func decodeResponse(response *http.Response) (result jsonMap, err error) {
	if response.StatusCode != 200 {
		err = errors.New(fmt.Sprintf("Strange status code: %v", response.Status))
		return
	}

	err = json.NewDecoder(response.Body).Decode(&result)
	return
}

func post(method string, request jsonMap) (result jsonMap, err error) {
	body, err := json.Marshal(request)
	if err != nil {
		return
	}

	response, err := http.Post(fmt.Sprintf(SLACK_API, method), "application/json", bytes.NewBuffer(body))
	if err != nil {
		return
	}

	result, err = decodeResponse(response)
	return
}

func postForm(method string, request jsonMap) (result jsonMap, err error) {
	var data url.Values = make(url.Values, len(request));

	for key := range request {
		data[key] = []string{request[key].(string)}
	}

	response, err := http.PostForm(fmt.Sprintf(SLACK_API, method), data)
	if err != nil {
		return
	}

	result, err = decodeResponse(response)
	return
}

func rtmStart(token string) (result string, err error) {
	log.Println("Authorize...")

	response, err := postForm("rtm.start", jsonMap{"token": interface{}(token)})
	if err != nil {
		return
	}

	if response["ok"].(bool) {
		log.Println("Response:", response)
		result = response["url"].(string)
	} else {
		err = errors.New(response["error"].(string))
	}
	return
}

func asyncRead(socket *websocket.Conn, queue chan jsonMap, quit chan bool) {
	log.Println("Listen...")

	var data jsonMap

	for {
		err := websocket.JSON.Receive(socket, &data)

		if err == io.EOF {
			quit <- true
			return
		} else if err != nil {
			log.Println(err)
		} else {
			log.Println("Recieved data:", data)
			queue <- data
		}
	}
}

func asyncAction(response jsonMap, queue chan jsonMap) {
	switch response["type"].(string) {
	case "hello":
		fmt.Println("Hello!")
	case "message":
		fmt.Println(
			response["user"].(string),
			"say:", response["text"].(string),
			"in channel:", response["channel"].(string))
	}
}

func main() {
	tokenPtr := flag.String("token", DUMMY_API_KEY, "Slack Bot API Token")
	flag.Parse()

	if (*tokenPtr == DUMMY_API_KEY) {
		flag.Usage()
		return
	}

	if url, err := rtmStart(*tokenPtr); err == nil {
		log.Println("Trying to connect webocket...")

		socket, err := websocket.Dial(url, "", "http://localhost/")
		if err != nil {
			log.Fatal(err)
		}

		var incomingQueue, outgoingQueue chan jsonMap = make(chan jsonMap), make(chan jsonMap)
		var quit chan bool = make(chan bool)

		go asyncRead(socket, incomingQueue, quit)

		ticker := time.NewTicker(10 * time.Second)
		go func() {
			for range ticker.C {
				outgoingQueue <- jsonMap{"type": "ping"}
			}
		}()

		for {
			select {
			case data := <-incomingQueue:
				go asyncAction(data, outgoingQueue)
			case data := <-outgoingQueue:
				log.Println("Send message:", data)
				err = websocket.JSON.Send(socket, data)
				if (err != nil) {
					log.Println("Message send failed: ", err)
					outgoingQueue <- data
				}
			case <-quit:
				fmt.Println("Quit")
				ticker.Stop()
				socket.Close()
				return
			}
		}
	} else {
		log.Fatal(err)
	}
}