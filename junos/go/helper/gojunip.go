package main

import (
	"os"
	"fmt"
	"net"
	"strings"
	"flag"
	"log"
	"time"
)

const port = "1234"
const capabilities = "getbgp:getalarm"
const server = "127.0.0.1"
const BUFF = 65536

func main() {
	getroute := flag.Bool("bgp", false, "query bgp route info")
	getalarm := flag.Bool("alarm", false, "query device chassis alarm")
        host := flag.String("d", "", "hostname or ip address")
        pfx := flag.String("r", "", "route prefix")
        test := flag.String("t", "", "test Asn index")
        list := flag.Bool("p", false, "print query-able device")
	help := flag.Bool("help", false, "print command usage")
        flag.Parse()
	flag.Usage = printusage
	if *help {
		printusage()
	}

        // get server availability and capability
	service := server + ":" + port
	addr, err := net.ResolveUDPAddr("udp4", service)
	if err != nil {
		printError(err)
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		printError(err)
	}
	timeout := 9 * time.Second
	conn.SetReadDeadline(time.Now().Add(timeout))
	//get server capa
	conn.Write([]byte("HELLO"))
	if err != nil {
	        printError(err)
	}

	var buf [BUFF]byte

	n, err := conn.Read(buf[0:])
	if err != nil {
		printError(err)
	}
	capa :=	checkCapabilites(buf[:n])

	if (*getroute  &&  strings.Contains(capa, "getbgp")) {
		if *test != "" {
			var buf  [BUFF]byte
			_, err = conn.Write([]byte("getbgp:test:" + *test))
			n, _ := conn.Read(buf[0:])
		        if err != nil {
				printError(err)
			}
			fmt.Println(string(buf[0:n]))
                }
		if *list {
			var buf [BUFF]byte
		       _, err = conn.Write([]byte("getbgp:list:"))
		       n, _ := conn.Read(buf[0:])
		       if err != nil {
			       printError(err)
	               }
		       fmt.Println(string(buf[0:n]))
		}
		if *pfx != "" {
			 _, _, err := net.ParseCIDR(*pfx)
			 if err != nil {
			         log.Fatal("Provide a valid IP route (ie x.x.x.x/n)")
			 }
			 if *host == "" {
				 log.Fatal("Provide a device name or ip to pull route from")
			}
			_, err = conn.Write([]byte("getbgp:route:" + *pfx + ":" + *host))
			if err != nil {
				printError(err)
			}
			for {
				var buf [BUFF]byte
			        n, _ := conn.Read(buf[0:])
				if n == 0 {
					break
				}
				if buf[n-1] == byte(004) {
					fmt.Println(string(buf[0:n-1]))
					break
				}
				fmt.Println(string(buf[0:n]))
			}
		}
	} else if (*getalarm  &&  strings.Contains(capa, "getalarm")) {
		var buf [BUFF]byte
		if *host == "" {
	                log.Fatal("Provide a device name or ip")
		}
		_, err = conn.Write([]byte("getalarm:" + *host))
		if err != nil {
	                printError(err)
	        }
		n, _ := conn.Read(buf[0:])
		if err != nil {
		        printError(err)
		}
		fmt.Println(string(buf[0:n]))
	} else {
                printusage()
	}

	os.Exit(0)
}

func printError(err error) {
	if netError, ok := err.(net.Error); ok && netError.Timeout() {
	        println(netError.Error())
		os.Exit(1)
	}
	switch t := err.(type) {
	        case *net.OpError:
			if t.Op == "dial" || t.Op == "read" {
				println(t.Err.Error() + " for remote address " + t.Addr.String())
			}
	}
	fmt.Fprintf(os.Stderr, "Fatal error %s", err.Error())
	os.Exit(1)
}

func checkCapabilites(buf []byte)(capa string) {
	s := string(buf)
	capa = strings.TrimLeft(s, "EHLLO:")
	return
}

func  printusage() {
	fmt.Println("gojunip.go usage :")
	fmt.Println("gojunip -bgp -t <AS number>                Resolve private as number")
	fmt.Println("gojunip -bgp -p                            Print query-able device")
	fmt.Println("gojunip -bgp -r <X.X.X.X/N> -d <device>    Query exact BGP route on device")
	fmt.Println("gojunip -alarm -d <device>                 Query device chassis alarm")
	os.Exit(0)
}
