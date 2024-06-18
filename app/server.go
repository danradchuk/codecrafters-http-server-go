package main

import (
	"fmt"
	"net"
	"os"
)

const empty200 = "HTTP/1.1 200 OK\r\n\r\n"
const empty400 = "HTTP/1.1 400 BAD REQUEST\r\n\r\n"
const empty404 = "HTTP/1.1 404 Not Found\r\n\r\n"

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Println("Logs from your program will appear here!")

	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221")
		os.Exit(1)
	}

	conn, err := l.Accept()
	if err != nil {
		fmt.Println("Error accepting connection: ", err.Error())
		os.Exit(1)
	}

	buf := make([]byte, 1024)
	nRead, err := conn.Read(buf)
	if err != nil {
		fmt.Println("Error reading the request: ", err.Error())
		os.Exit(1)
	}

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
				start = i
			} else {
				end = i
				break
			}
		}
		i++
	}

	url := buf[start:end]
	fmt.Printf("%s\n", string(buf))
	if string(url) == "/" {
		conn.Write([]byte(empty200))
	} else {
		conn.Write([]byte(empty404))
	}

	// nWrite, err := conn.Write([]byte(empty200))
	// if err != nil {
	// 	fmt.Println("Error writing the response: ", err.Error())
	// 	os.Exit(1)
	// }
}
