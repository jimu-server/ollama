package sdk

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/format"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

var pool *sync.Pool

var host, port = "127.0.0.1", "11434"

func init() {
	// 初始化 sdk
	var err error
	host, port, err = net.SplitHostPort(strings.Trim(os.Getenv("OLLAMA_HOST"), "\"'"))
	if err != nil {
		host, port = "127.0.0.1", "11434"
		if ip := net.ParseIP(strings.Trim(os.Getenv("OLLAMA_HOST"), "[]")); ip != nil {
			host = ip.String()
		}
	}
	pool = &sync.Pool{New: func() interface{} {
		return &http.Client{}
	}}
}

type StreamData struct {
	Data []byte
	Err  error
	Resp *http.Response
}
type DataEvent <-chan StreamData

type OllamaSdk struct {
}

type Message struct {
	Role    string
	Content string
}

type Request struct {
	Model    string  `json:"model"`
	Message  Message `json:"message"`
	Messages []Message
}

func Chat(req *api.ChatRequest) (DataEvent, error) {
	client := pool.Get().(*http.Client)
	var err error
	var request *http.Request
	marshal, err := jsoniter.Marshal(req)
	if err != nil {
		return nil, err
	}
	buffer := bytes.NewBuffer(marshal)
	url := fmt.Sprintf("http://%s:%s/api/chat", host, port)
	request, err = http.NewRequest(http.MethodPost, url, buffer)
	if err != nil {
		return nil, err
	}
	do, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	return chat(do)
}

func chat(response *http.Response) (DataEvent, error) {
	send := make(chan StreamData, 100)
	go func() {
		var err error
		var buf []byte
		// 使用 bufio.NewReader 创建一个读取器，方便按行读取
		scanner := bufio.NewScanner(response.Body)
		scanBuf := make([]byte, 0, 512*format.KiloByte)
		scanner.Buffer(scanBuf, 512*format.KiloByte)
		// 创建一个通道
		defer close(send)
		for scanner.Scan() {
			buf = scanner.Bytes()
			if err = scanner.Err(); err == io.EOF {
				break // 文件结束
			} else if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe) {
				send <- StreamData{Data: nil, Err: errors.New("END")}
				break
			} else if err != nil {
				send <- StreamData{Data: nil, Err: err}
				break
			}
			buffer := bytes.NewBuffer(buf)
			buffer.WriteString("\n")
			send <- StreamData{Data: buffer.Bytes(), Err: nil, Resp: response}
		}
	}()
	return send, nil
}
