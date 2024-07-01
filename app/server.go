package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

const Empty200 = "HTTP/1.1 200 OK\r\n\r\n"
const Empty201 = "HTTP/1.1 201 Created\r\n\r\n"
const Empty400 = "HTTP/1.1 400 Bad Request\r\n\r\n"
const Empty404 = "HTTP/1.1 404 Not Found\r\n\r\n"
const Empty405 = "HTTP/1.1 405 Method Not Allowed\r\n\r\n"

var statusCodeToStatusStr = map[int]string{
	200: "OK",
	201: "Created",
	400: "Bad Request",
	404: "Not Found",
	405: "Method Not Allowed",
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

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Println("logs from your program will appear here!")

	var directory = flag.String("directory", "default", "a string for directory flag")
	flag.Parse()

	var listenConfig net.ListenConfig
	l, err := listenConfig.Listen(context.Background(), "tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("failed to bind to port 4221")
		os.Exit(1)
	}
	defer l.Close()

	var successCount = 0
	var errChan = make(chan error)
	var doneChan = make(chan string)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		for {
			// accept
			conn, err := l.Accept()
			if err != nil {
				fmt.Printf("error accepting connection: %s", err.Error())
				continue
			}

			go handleConnection(conn, *directory, errChan, doneChan)

			select {
			case error := <-errChan:
				fmt.Printf("error occured: %s\n", error)
			case success := <-doneChan:
				fmt.Printf("a request %d is processed %s \n", successCount, success)
				successCount++
			}
		}
	}()

	<-sigCh
	// graceful shutdown https://eli.thegreenplace.net/2020/graceful-shutdown-of-a-tcp-server-in-go/

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	<-ctx.Done()
	log.Println("timeout of 5 seconds")

	log.Println("service was shut down")

}

func handleConnection(conn net.Conn, directory string, errChan chan<- error, doneChan chan<- string) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	statusLine, err := reader.ReadString('\n')
	if err != nil {
		errChan <- err
		return
	}

	// extract request line
	statusLineParts := strings.Split(statusLine, " ")
	method := statusLineParts[0]
	path := statusLineParts[1]

	// extract headers
	var headers = make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			errChan <- err
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
	switch path {
	case "/":
		_, err := conn.Write([]byte(Empty200))
		if err != nil {
			errChan <- err
			return
		}
	case "/echo":
		encodings := strings.Split(headers["Accept-Encoding"], ",")

		var encoding string
		for _, e := range encodings {
			e := strings.TrimSpace(e)
			if e == "gzip" {
				encoding = e
				break
			}
		}

		body := strings.Split(path, "/")[2]
		if encoding == "gzip" {
			_, err := writeResponse(conn, 200, []byte(body), "text/plain", true, encoding)
			if err != nil {
				errChan <- err
				return
			}
		} else {
			_, err := writeResponse(conn, 200, []byte(body), "text/plain", false, "")
			if err != nil {
				errChan <- err
				return
			}
		}
	case "/user-agent":
		var userAgent = headers["User-Agent"]
		writeResponse(conn, 200, []byte(userAgent), "text/plain", false, "")
	case "/file":
		fileName := strings.Split(path, "/")[2]

		contentLength, err := strconv.Atoi(headers["Content-Length"])
		if err != nil {
			errChan <- err
			return
		}

		buf := make([]byte, contentLength)
		_, err = io.ReadFull(reader, buf)
		if err != nil {
			errChan <- err
			return
		}

		if method == "GET" {
			handleFileGet(conn, directory, fileName)
		} else if method == "POST" {
			handleFilePost(conn, directory, fileName, buf)
		} else {
			conn.Write([]byte(Empty405))
		}
	default:
		_, err := conn.Write([]byte(Empty404))
		if err != nil {
			errChan <- err
			return
		}
	}

	doneChan <- "success"
}

func handleFileGet(conn net.Conn, directory string, fileName string) {
	filePath := directory + "/" + fileName
	f, err := os.Open(filePath)
	if err != nil {
		conn.Write([]byte(Empty404))
		return
	}
	defer f.Close()

	b, err := os.ReadFile(filePath)
	if err != nil {
		conn.Write([]byte(Empty404))
		return
	}
	writeResponse(conn, 200, b, "application/octet-stream", false, "")
}

func handleFilePost(conn net.Conn, directory string, fileName string, content []byte) {
	filePath := directory + "/" + fileName
	f, err := os.Create(filePath)
	if err != nil {
		return
	}
	defer f.Close()

	_, err = f.Write(content)
	if err != nil {
		return
	}

	conn.Write([]byte(Empty201))
}

// func extractHeader(headers []string) string {
// 	var header string
// 	for _, h := range headers {
// 	}
//
// 	return header
// }

func writeResponse(w io.Writer, statusCode int, body []byte, contentType string, compressed bool, encoding string) (int, error) {
	esb := &errStringBuilder{sb: strings.Builder{}}

	s := fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, statusCodeToStatusStr[statusCode])
	esb.writeStr(s)

	contentType = fmt.Sprintf("Content-Type: %s\r\n", contentType)
	esb.writeStr(contentType)

	if compressed {
		e := fmt.Sprintf("Content-Encoding: %s\r\n", encoding)
		esb.writeStr(e)

		b, err := compress(body)
		if err != nil {
			return 0, err
		}
		body = b
	}

	contentLength := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	esb.writeStr(contentLength)

	esb.write(body)

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
