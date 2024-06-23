package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

const empty200 = "HTTP/1.1 200 OK\r\n\r\n"
const empty400 = "HTTP/1.1 400 BAD REQUEST\r\n\r\n"
const empty404 = "HTTP/1.1 404 Not Found\r\n\r\n"

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Println("logs from your program will appear here!")

	// listen
	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("failed to bind to port 4221")
		os.Exit(1)
	}

	// accept
	conn, err := l.Accept()
	if err != nil {
		fmt.Printf("error accepting connection: %s", err.Error())
		os.Exit(1)
	}

	// read a request
	buf := make([]byte, 1024)
	nRead, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("error reading the request: %s", err.Error())
		os.Exit(1)
	}

	// respond if the body is empty
	if nRead == 0 {
		conn.Write([]byte(empty400))
		os.Exit(1)
	}

	// extract URL
	var i int
	var start int
	var end int
	for i != nRead {
		if buf[i] == ' ' {
			if start == 0 {
				start = i + 1
			} else {
				end = i
				break
			}
		}
		i++
	}

	// simple router
	url := string(buf[start:end])
	path := strings.Split(url, "/")
	fmt.Printf("%v\n", path)

	if url == "/" {
		_, err := conn.Write([]byte(empty200))
		if err != nil {
			fmt.Printf("error writing the response to /: %s", err.Error())
			os.Exit(1)
		}
	} else if path[1] == "echo" {
		_, err := writeOK(conn, []byte(path[2]))
		if err != nil {
			fmt.Printf("error writing the response to /echo: %s ", err.Error())
			os.Exit(1)
		}
	} else if path[1] == "user-agent" {
		var i int
		// skip request line
		for i != nRead {
			if buf[i] == '\r' {
				i += 2 // skip \r\n up to Headers
				break
			}
			i++
		}

		// extract headers
		var j = i
		for j != nRead {
			if buf[j] == '\r' && buf[j+2] == '\r' {
				j += 2
				break
			}
			j++
		}

		var userAgent string
		headers := strings.Split(string(buf[i:j]), "\r\n")
		for _, h := range headers {
			if strings.HasPrefix(h, "User-Agent:") {
				userAgent = strings.TrimSuffix(strings.TrimPrefix(h, "User-Agent: "), "\r\n")
				break
			}
		}

		writeOK(conn, []byte(userAgent))
	} else {
		_, err := conn.Write([]byte(empty404))
		if err != nil {
			fmt.Printf("error: unknown path %s", err.Error())
			os.Exit(1)
		}
	}

	// nWrite, err := conn.Write([]byte(empty200))
	// if err != nil {
	// 	fmt.Println("Error writing the response: ", err.Error())
	// 	os.Exit(1)
	// }
}

func writeOK(w io.Writer, body []byte) (int, error) {
	var resp strings.Builder

	resp.WriteString("HTTP/1.1 200 OK\r\n")

	contentType := "Content-Type: text/plain\r\n"
	resp.WriteString(contentType)

	contentLength := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	resp.WriteString(contentLength)

	resp.Write(body)

	return w.Write([]byte(resp.String()))
}
