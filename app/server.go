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

	var b []byte
	nRead, err := conn.Read(b)
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
		if b[i] == ' ' {
			if start == 0 {
				start = i
			} else {
				end = i
				break
			}
		}
		i++
	}

	url := b[start:end]
	if len(url) == 1 {
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
