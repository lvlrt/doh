package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"io/ioutil"
	"encoding/json"
	"strconv"
	"strings"
	"runtime"
	"os/exec"
)

/* A Simple function to verify error */
func CheckError(err error) {
	if err  != nil {
		fmt.Println("Error: " , err)
		os.Exit(0)
	}
}

func main() {
	if os.Geteuid() != 0 { //check if not root
		if runtime.GOOS == "darwin" { // macos
			command := "osascript -e 'do shell script \""+os.Args[0]+"\" with prompt \"DoH needs system rights\" with administrator privileges'"
			cmd := exec.Command("sh", "-c", command)
			stdoutStderr, err := cmd.CombinedOutput()
			//TODO detect failed permissions grant -> error with; 'Doh needs system permissions to run'
			os.Exit(0)
		} else if runtime.GOOS == "windows" { //windows
			//TODO test if this works?
			cmd := exec.Command("/runas", "/profile", "/user:administrator", os.Args[0])
			stdoutStderr, err := cmd.CombinedOutput()
			//TODO detect failed permissions grant -> error with; 'Doh needs system permissions to run'
			os.Exit(0)
		} else { //linux
			fmt.Println("Please run this program as root")
		}
		os.Exit(126)
	}


	ServerAddr,err := net.ResolveUDPAddr("udp",":53")
	CheckError(err)

	ServerConn, err := net.ListenUDP("udp", ServerAddr)
	CheckError(err)
	defer ServerConn.Close()

	for {
		buf := make([]byte, 1024)
		n,addr,_ := ServerConn.ReadFromUDP(buf)

		go func(n int, addr *net.UDPAddr, buf []byte) {
			//REQUEST
			query := []byte{}
			for _, element := range buf[13:] {
				if element == 0 {
					break
				} else {
					query = append(query, element)
				}
			}

			record := ""
			for _, element := range query {
				if element < 31 { //discover dots 
					record+="."
				} else {
					record+=string(element)
				}
			}

			//QUERY OVER HTTPS
			res, err := http.Get("https://1.1.1.1/dns-query?ct=application/dns-json&name="+record+"&type=A")
			CheckError(err)
			body, err := ioutil.ReadAll(res.Body)
			CheckError(err)

			//decode JSON response in body
			var f interface{}
			json.Unmarshal(body, &f)
			m := f.(map[string]interface{}) //make a mappable opbject
			resolved_query := m["Answer"].([]interface{})[0].(map[string]interface{})// contains TTL name data type
			//RESPONSE
			response := buf[:2] //id
			response = append(response, []byte{129, 128}...) //flags
			response = append(response, []byte{0, 1, 0, 1, 0, 0, 0, 0}...) //rr
			response = append(response, 3) //start response
			response = append(response, query...) //query
			response = append(response, 0) //query end
			response = append(response, []byte{0, 1, 0, 1}...) //type and class
			response = append(response, []byte{192, 12}...) //first record
			response = append(response, []byte{0, 1, 0, 1}...) //type class first record
			//ttl
			ttl := strconv.FormatInt(int64(resolved_query["TTL"].(float64)), 2) //make binary 
			ttl = strings.Repeat("0", 32-len(ttl)) + ttl //prepend 0's 
			for _, element := range []int{0,8,16,24} { //split complete 32-bit in byte-sized chunks
				i, _ :=strconv.ParseInt(ttl[element:element+8],2,64) //derive value from binary string
				response = append(response, byte(i)) //add to response
			}
			//data
			response = append(response, []byte{0, 4}...) //TODO temp harcode data length (4)
			for _, element := range strings.Split(resolved_query["data"].(string), ".") { //every part of ip to decimal
				i, _ := strconv.Atoi(element) //convert element to int
				response = append(response, byte(i)) //add to response
			}
			_, err = ServerConn.WriteToUDP(response, addr)
			CheckError(err)

			//TODO implement type 
			//TODO catch empty response (failed)
		}(n,addr,buf)
	}
}
