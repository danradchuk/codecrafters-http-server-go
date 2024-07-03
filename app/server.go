package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
)

const (
	Empty200 = "HTTP/1.1 200 OK\r\n\r\n"
	Empty201 = "HTTP/1.1 201 Created\r\n\r\n"
	Empty400 = "HTTP/1.1 400 Bad Request\r\n\r\n"
	Empty404 = "HTTP/1.1 404 Not Found\r\n\r\n"
	Empty405 = "HTTP/1.1 405 Method Not Allowed\r\n\r\n"
	Empty500 = "HTTP/1.1 405 Internal Server Error\r\n\r\n"
)

var statusCodeToStatusStr = map[int]string{
	200: "OK",
	201: "Created",
	400: "Bad Request",
	404: "Not Found",
	405: "Method Not Allowed",
}

type server struct {
	listener net.Listener
	quit     chan interface{}
	wg       sync.WaitGroup
}

func newServer(addr string, directory string, transferErrChan chan<- error, doneChan chan<- string) *server {
	s := &server{
		quit: make(chan interface{}),
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	s.listener = l
	s.wg.Add(1)

	go s.serve(directory, transferErrChan, doneChan)

	return s
}
func (s *server) serve(directory string, transferErrChan chan<- error, doneChan chan<- string) {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				log.Println("accept error", err)
			}
		} else {
			s.wg.Add(1)
			go func() {
				handleConnection(conn, directory, transferErrChan, doneChan)
				s.wg.Done()
			}()
		}
	}
}
func (s *server) stop() {
	log.Println("service is shutting down")
	close(s.quit)
	s.listener.Close()
	s.wg.Wait()
	log.Println("service was shut down")
}

type response struct {
	status      int
	body        []byte
	compressed  bool
	encoding    string
	contentType string
}

func main() {
	fmt.Println("logs from your program will appear here!")

	var directory = flag.String("directory", ".", "a string for directory flag")
	flag.Parse()

	var successCount = 0
	var transferErrChan = make(chan error)
	var doneChan = make(chan string)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		for {
			select {
			case err := <-transferErrChan:
				fmt.Printf("an error has occured: %s\n", err)
			case success := <-doneChan:
				successCount++
				fmt.Printf("request № %d has processed %s \n", successCount, success)
			}
		}
	}()

	s := newServer("0.0.0.0:4221", *directory, transferErrChan, doneChan)
	<-sigCh
	s.stop()
}

func handleConnection(conn net.Conn, directory string, transferErrChan chan<- error, doneChan chan<- string) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// extract status line
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		_, err := conn.Write([]byte(Empty400))
		if err != nil {
			transferErrChan <- err
			return
		}
		return
	}

	statusLineParts := strings.Split(statusLine, " ")
	method := statusLineParts[0]
	path := strings.Split(statusLineParts[1], "/")

	// extract headers
	var headers = make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_, err := conn.Write([]byte(Empty400))
			if err != nil {
				transferErrChan <- err
				return
			}
			return
		}

		if line == "\r\n" {
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	// simple router
	var transferErr error
	switch path[1] {
	case "":
		_, transferErr = writeResponse(conn, 200, nil, "text/plain", false, "")
	case "echo":
		resp := handleEcho(headers, path)
		_, transferErr = writeResponse(conn, resp.status, resp.body, resp.contentType, resp.compressed, resp.encoding)
	case "user-agent":
		_, transferErr = writeResponse(conn, 200, []byte(headers["User-Agent"]), "text/plain", false, "")
	case "files":
		resp := handleFileOps(directory, path, method, headers, reader)
		_, transferErr = writeResponse(conn, resp.status, resp.body, resp.contentType, resp.compressed, resp.encoding)
	default:
		_, transferErr = writeResponse(conn, 404, nil, "text/plain", false, "")
	}

	if transferErr != nil {
		transferErrChan <- transferErr
	} else {
		doneChan <- "success"
	}
}

func handleEcho(headers map[string]string, path []string) *response {
	encodings := strings.Split(headers["Accept-Encoding"], ",")

	var encoding string
	for _, e := range encodings {
		e := strings.TrimSpace(e)
		if e == "gzip" {
			encoding = e
			break
		}
	}

	body := path[2]
	if encoding == "gzip" {
		return &response{
			status:      200,
			body:        []byte(body),
			compressed:  true,
			encoding:    encoding,
			contentType: "text/plain",
		}
	} else {
		return &response{
			status:      200,
			body:        []byte(body),
			contentType: "text/plain",
		}
	}
}

func handleFileOps(directory string, path []string, method string, headers map[string]string, reader *bufio.Reader) *response {
	fileName := path[2]

	// could be switch/case but it'll be non-exhaustive
	if method == "GET" {
		return handleFileGet(directory, fileName)
	} else if method == "POST" {
		contentLength, err := strconv.Atoi(headers["Content-Length"])
		if err != nil {
			return &response{status: 400}
		}

		buf := make([]byte, contentLength)
		_, err = io.ReadFull(reader, buf)
		if err != nil {
			return &response{status: 500}
		}

		return handleFilePost(directory, fileName, buf)
	}

	return &response{status: 405}
}
func handleFileGet(directory string, fileName string) *response {
	filePath := directory + "/" + fileName
	f, err := os.Open(filePath)
	if err != nil {
		return &response{status: 404}
	}
	defer f.Close()

	b, err := os.ReadFile(filePath)
	if err != nil {
		return &response{status: 404}
	}

	return &response{
		status:      200,
		body:        b,
		contentType: "application/octet-stream",
	}
}
func handleFilePost(directory string, fileName string, content []byte) *response {
	filePath := directory + "/" + fileName
	f, err := os.Create(filePath)
	if err != nil {
		return &response{status: 404}
	}
	defer f.Close()

	_, err = f.Write(content)
	if err != nil {
		return &response{status: 500}
	}

	return &response{status: 201}
}

type errStringBuilder struct {
	sb  strings.Builder
	err error
}

func (esb *errStringBuilder) writeStr(s string) {
	if esb.err != nil {
		return
	}

	_, esb.err = esb.sb.WriteString(s)
}
func (esb *errStringBuilder) write(buf []byte) {
	if esb.err != nil {
		return
	}

	_, esb.err = esb.sb.Write(buf)
}
func writeResponse(w io.Writer, statusCode int, body []byte, contentType string, compressed bool, encoding string) (int, error) {
	esb := &errStringBuilder{sb: strings.Builder{}}

	s := fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, statusCodeToStatusStr[statusCode])
	esb.writeStr(s)

	if contentType != "" {
		contentType = fmt.Sprintf("Content-Type: %s\r\n", contentType)
		esb.writeStr(contentType)
	}

	if compressed {
		e := fmt.Sprintf("Content-Encoding: %s\r\n", encoding)
		esb.writeStr(e)

		b, err := compress(body)
		if err != nil {
			return 0, err
		}
		body = b
	}

	if body != nil {
		contentLength := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
		esb.writeStr(contentLength)
		esb.write(body)
	} else {
		esb.writeStr("\r\n")
	}

	return w.Write([]byte(esb.sb.String()))
}
func compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	w := gzip.NewWriter(&buf)
	_, err := w.Write(data)
	if err != nil {
		return nil, err
	}

	err = w.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
