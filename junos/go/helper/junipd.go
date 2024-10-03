package main

import (
	"encoding/xml"
	"fmt"
	"github.com/Juniper/go-netconf/netconf"
	"golang.org/x/crypto/ssh"
	"github.com/alouca/gosnmp"
	"log"
	"net"
	"strings"
)

var devname = map[string]string{
	"64512": "dev-hostname1", "64513": "dev-hostname2", "64514": "dev-hostname3",
}

var devsite = []string{
	"dev-hostname1", "dev-hostname2", "devhostname3",
}

var site = []string{"id1", "id2", "id3", "id4"}

// Route struct models the xml representation of show route cde
type Route struct {
	XMLName xml.Name   `xml:"rpc-reply"`
	Table   string     `xml:"route-information>route-table>table-name"`
	Rt      RouteEntry `xml:"route-information>route-table>rt"`
}

type RouteEntry struct {
	Dest  string `xml:"rt-destination"`
	Entry []struct {
		Proto       string   `xml:"protocol-name"`
		Age         string   `xml:"age"`
		AsPath      string   `xml:"as-path"`
		Communities []string `xml:"communities>community"`
	} `xml:"rt-entry"`
}

type GetAlarm struct {
	XMLName xml.Name     `xml:"rpc-reply"`
	Alarm   ChassisAlarm `xml:"alarm-information"`
}

type ChassisAlarm struct {
	AlarmSum    string `xml:"alarm-summary>active-alarm-count"`
	AlarmDetail []struct {
	        Atime AlarmTime `xml:"alarm-time"`
	        Desc  string    `xml:"alarm-description"`
	} `xml:"alarm-detail"`
}

type AlarmTime struct {
        Epoch uint32 `xml:"seconds,attr"`
        Date  string `xml:",chardata"`
}

const servport = ":1234"
const capabilities = "getbgp:getalarm"

const netconfport = ":22"
const user = "younameit"
const sym = "blurr"

const oid = "1.3.6.1.4.1.2636.3.4.2.3.1.0"
const cty = "blurr"

func main() {
	// listen to incoming udp packets
	service, _ := net.ResolveUDPAddr("udp4", servport)
	pc, err := net.ListenUDP("udp4", service)
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()

	for {
		buf := make([]byte, 1500)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			continue
		}
		go serve(*pc, addr, buf[:n])
	}

}

func serve(pc net.UDPConn, addr net.Addr, buf []byte) {
	s := string(buf)
	if s[:5] == "HELLO" {
		pc.WriteTo([]byte("EHLLO:"+capabilities), addr)
		return
	} else if strings.Contains(s, "getbgp:test") {
		cli := strings.Split(s, ":")
		if elem, ok := devname[cli[2]]; ok {
			pc.WriteTo([]byte("Device name for Asn "+cli[2]+" is "+elem), addr)
		} else {
			pc.WriteTo([]byte("No device for Asn "+cli[2]+" in database"), addr)
		}
		return
	} else if strings.Contains(s, "getbgp:list") {
		var temp []byte
		for i := range site {
			s := fmt.Sprint("\n" + site[i] + " : \n")
			temp = append(temp, s...)
			for j := range devsite {
				if strings.Contains(devsite[j], site[i]) {
					s := fmt.Sprint("    " + devsite[j] + "\n")
					temp = append(temp, s...)
				}
			}
		}
		pc.WriteTo(temp, addr)
		return
	} else if strings.Contains(s, "getbgp:route") {
		cli := strings.Split(s, ":")
		// netconf initialization & connection
		sshConfig := &ssh.ClientConfig{
			User:            user,
			Auth:            []ssh.AuthMethod{ssh.Password(sym)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
		var hostname string = cli[3] + netconfport
		conn, err := netconf.DialSSH(hostname, sshConfig)
		if err != nil {
			pc.WriteTo([]byte(err.Error()), addr)
			return
		}
		var rpc_cde string = "<get-route-information><destination>" + cli[2] + "</destination><match>exact</match><protocol>bgp</protocol><detail/></get-route-information>"
		reply, err := conn.Exec(netconf.RawMethod(rpc_cde))
		if err != nil {
			pc.WriteTo([]byte("Error processing rpc cde ; "+err.Error()), addr)
			conn.Close()
			return
		}
		var q Route
		err = xml.Unmarshal([]byte(reply.RawReply), &q)
		if q.Table == "" {
			pc.WriteTo([]byte(cli[3]+" : No route found for prefix "+cli[2]+"\n"), addr)
			conn.Close()
			return
		}

		var temp []byte
		s := fmt.Sprintf("%s : %d routes in routing table %s for prefix %s\n\n", cli[3], len(q.Rt.Entry), q.Table, cli[2])
		temp = append(temp, s...)
		for i := 0; i < len(q.Rt.Entry); i++ {
			var origin string
			var ok bool
			var comm string
			var asfmt string // a formatted as path
			if q.Rt.Entry[i].AsPath != "" {
				asfmt = strings.Replace(q.Rt.Entry[i].AsPath, "AS path: Recorded\n", "", 1) // get rid of useless info
				asfmt = strings.Replace(asfmt, "AS path: ", "", 1)
				asfmt = strings.Replace(asfmt, "65501", "", -1)        // remove generic 65501 as
				res := strings.Fields(strings.TrimRight(asfmt, "I\n")) // remove trailing as attribute
				if len(res) > 0 {
					lastas := res[len(res)-1]
					if origin, ok = devname[lastas]; !ok {
						origin = "Device not in db !"
					}
				} else {
					origin = cli[3]
				}
			}
			if len(q.Rt.Entry[i].Communities) > 0 {
				comm = strings.Join(q.Rt.Entry[i].Communities, " ")
			}
			temp = append(temp, fmt.Sprintf("  Protocol        : %s\n    Age           : %s\n    As path       : %s    Origin device : %s\n    Communities   : %s\n\n",
				q.Rt.Entry[i].Proto, q.Rt.Entry[i].Age, asfmt, origin, comm)...)
		} //end for
		pc.WriteTo(append(temp, 004), addr) // add end of transmission at end of data, as route info could exceed 1500 byte
		conn.Close()
		return
	} else if strings.Contains(s, "getalarm") {
		cli := strings.Split(s, ":")
		var hostname string = cli[1]
		var temp []byte
		var isred int = 0
		var ok bool

		sock, err := gosnmp.NewGoSNMP(hostname, cty, gosnmp.Version2c, 3)
		resp, err := sock.Get(oid)

		if err != nil {
			pc.WriteTo([]byte("Fatal : "+err.Error()), addr)
			return
		} else if isred, ok = resp.Variables[0].Value.(int); !ok {
			pc.WriteTo([]byte("Fatal : snmp return value invalid"), addr)
			return
		} else if isred != 3 { // 1:state unknown 2:state off 3:state on
			pc.WriteTo([]byte("No critical alarm on " + hostname), addr)
			return
		} else if isred == 3 {
			sshConfig := &ssh.ClientConfig{
				User:            user,
				Auth:            []ssh.AuthMethod{ssh.Password(sym)},
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			}
			conn, err := netconf.DialSSH(hostname + netconfport, sshConfig)
			if err != nil {
				pc.WriteTo([]byte(err.Error()), addr)
				return
			}
			var rpc_cde string = "<get-alarm-information/>"
			reply, err := conn.Exec(netconf.RawMethod(rpc_cde))
			if err != nil {
				pc.WriteTo([]byte("Error processing rpc cde ; " + err.Error()), addr)
				conn.Close()
				return
			}
			var q GetAlarm
			err = xml.Unmarshal([]byte(reply.RawReply), &q)
			temp = append(temp, fmt.Sprintf("Alarm summary       : %s\n", q.Alarm.AlarmSum)...)
			for i, _ := range q.Alarm.AlarmDetail {
			        temp = append(temp, fmt.Sprintf("Alarm[%d] epoch      : %d\n  Alarm date        : %v\n  Alarm description : %s\n", i + 1,
				    q.Alarm.AlarmDetail[i].Atime.Epoch, strings.Trim(q.Alarm.AlarmDetail[i].Atime.Date, "\r\n"), q.Alarm.AlarmDetail[i].Desc)...)
			}
			pc.WriteTo(temp, addr)
			conn.Close()
			return
		}
	} else {
		pc.WriteTo([]byte("Invalid command please refer to gojunip.go --help for syntax usage"), addr)
		return
	}
} //end func
