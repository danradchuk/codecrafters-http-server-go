package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

const empty200 = "HTTP/1.1 200 OK\r\n\r\n"
const empty201 = "HTTP/1.1 201 Created\r\n\r\n"
const empty400 = "HTTP/1.1 400 BAD REQUEST\r\n\r\n"
const empty404 = "HTTP/1.1 404 Not Found\r\n\r\n"
const empty405 = "HTTP/1.1 405 Method Not Allowed\r\n\r\n"

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Println("logs from your program will appear here!")

	var directory = flag.String("directory", "default", "a string for directory flag")
	flag.Parse()

	// listen
	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("failed to bind to port 4221")
		os.Exit(1)
	}
	defer l.Close()

	for {
		// accept
		conn, err := l.Accept()
		if err != nil {
			fmt.Printf("error accepting connection: %s", err.Error())
			os.Exit(1)
		}

		go handleConnection(conn, *directory)
	}

}

func handleConnection(conn net.Conn, directory string) {
	defer conn.Close()

	// read a request
	req := make([]byte, 1024)
	nRead, err := conn.Read(req)
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
		if req[i] == ' ' {
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
	url := string(req[start:end])
	path := strings.Split(url, "/")
	fmt.Printf("%v\n", path)

	if url == "/" {
		_, err := conn.Write([]byte(empty200))
		if err != nil {
			fmt.Printf("error writing the response to /: %s", err.Error())
			os.Exit(1)
		}
	} else if path[1] == "echo" {
		encoding := extractHeader(req, nRead, "Accept-Encoding")
		if encoding != "gzip" {
			_, err := write2xx(conn, 200, []byte(path[2]), "text/plain", false, "")
			if err != nil {
				fmt.Printf("error writing the response to /echo: %s ", err.Error())
				os.Exit(1)
			}
		} else {
			_, err := write2xx(conn, 200, []byte(path[2]), "text/plain", true, encoding)
			if err != nil {
				fmt.Printf("error writing the response to /echo: %s ", err.Error())
				os.Exit(1)
			}
		}
	} else if path[1] == "user-agent" {
		var userAgent = extractHeader(req, nRead, "User-Agent")
		write2xx(conn, 200, []byte(userAgent), "text/plain", false, "")
	} else if path[1] == "files" {
		// parse method
		var i int
		for i != nRead {
			if req[i] == ' ' {
				break
			}
			i++
		}

		method := string(req[0:i])
		if method == "GET" {
			handleFileGet(conn, directory, path[2])
		} else if method == "POST" {
			var i int
			for i != nRead {
				if req[i] == '\r' && req[i+2] == '\r' {
					i += 4
					break
				}
				i++
			}
			handleFilePost(conn, directory, path[2], req[i:nRead])
		} else {
			conn.Write([]byte(empty405))
		}
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

func write2xx(w io.Writer, status int, body []byte, contentType string, compressed bool, encoding string) (int, error) {
	var resp strings.Builder

	s := fmt.Sprintf("HTTP/1.1 %d OK\r\n", status)
	resp.WriteString(s)

	t := fmt.Sprintf("Content-Type: %s\r\n", contentType)
	resp.WriteString(t)

	if compressed {
		e := fmt.Sprintf("Content-Encoding: %s\r\n", encoding)
		resp.WriteString(e)
	}

	contentLength := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	resp.WriteString(contentLength)

	resp.Write(body)

	return w.Write([]byte(resp.String()))
}

func handleFileGet(conn net.Conn, directory string, fileName string) {
	filePath := directory + "/" + fileName
	println(directory)
	f, err := os.Open(filePath)
	if err != nil {
		conn.Write([]byte(empty404))
		return
	}
	defer f.Close()

	b, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("can't read the file %s. error: %s\n", filePath, err.Error())
		os.Exit(1)
	}
	write2xx(conn, 200, b, "application/octet-stream", false, "")
}

func handleFilePost(conn net.Conn, directory string, fileName string, content []byte) {
	filePath := directory + "/" + fileName
	f, err := os.Create(filePath)
	if err != nil {
		return
	}
	defer f.Close()

	nWritten, err := f.Write(content)
	if err != nil {
		return
	}
	println(nWritten)

	conn.Write([]byte(empty201))
}

func extractHeader(req []byte, nRead int, name string) string {

	var i int
	// skip request line
	for i != nRead {
		if req[i] == '\r' {
			i += 2 // skip \r\n up to Headers
			break
		}
		i++
	}

	// extract headers
	var j = i
	for j != nRead {
		if req[j] == '\r' && req[j+2] == '\r' {
			j += 2
			break
		}
		j++
	}

	headers := strings.Split(string(req[i:j]), "\r\n")
	prefix := fmt.Sprintf("%s: ", name)

	var header string
	for _, h := range headers {
		if strings.HasPrefix(h, name) {
			header = strings.TrimSuffix(strings.TrimPrefix(h, prefix), "\r\n")
			break
		}
	}

	return header
}
