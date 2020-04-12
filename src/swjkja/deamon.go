package swjkja

import (
	"bufio"
	"errors"
	"fmt"
	"logging"
	"net"
	"os"
	"rand"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type server_info struct {
	ip            *net.UDPAddr
	lastHeartbeat time.Time
	hostname      string
	clients       int
	sv_maxclients int
	gametype      int
	mapname       string
	fdisable      int
	truejedi      int
	protocol      int
	challenge     string
	status        int // 0 - without info, 1 - pending info, 2 - ready
}

type event struct {
	t       int // 0 - error, 1 - shutdown , 2 - info, 3 - warning
	sender  chan event
	err     error
	payload string
}

type list_request struct {
	t       int // 0 - heartbeat, 2 - infoResponse, 3 - getServers
	addr    *net.UDPAddr
	payload []string
}

const MSG_MAXLEN = 1024
const MAX_INFOSTRING = 1024
const MAX_SERVERS = 1024
const SERVER_TIMEOUT = 300 // sec

const (
	EVENT_ERROR = iota
	EVENT_SHUTDOWN
	EVENT_LOG_INFO
	EVENT_LOG_WARNING
)

const (
	REQUEST_HEARTBEAT = iota
	REQUEST_INFORESPONSE
	REQUEST_GETSERVERS
)

var server_list = make([]server_info, 0, MAX_SERVERS)
var socket *net.UDPConn

var channel_event = make(chan event)
var channel_command = make(chan event)
var channel_udp = make(chan event)
var channel_serverlist = make(chan event)

var channel_listrequest = make(chan list_request)

func sendUdpMessage(ip *net.UDPAddr, data []byte) error {
	var buffer []byte = make([]byte, 4)
	buffer[0] = 255
	buffer[1] = 255
	buffer[2] = 255
	buffer[3] = 255

	buffer = append(buffer, data...)
	_, err := socket.WriteToUDP(buffer, ip)
	if err != nil {
		return err
	}
	return nil
}

func infoStringToMap(data string) (map[string]string, error) {
	if len(data) > MAX_INFOSTRING {
		return nil, errors.New("MAX_INFOSTRING limit exceeded\n")
	}
	if (strings.Index(data, "\\")) == 0 {
		data = data[1:len(data)]
	}
	ret := make(map[string]string)
	raw := strings.Split(data, "\\")
	ln := len(raw)
	for i := 0; i < ln; i = i + 2 {
		a := raw[i : i+2]
		ret[a[0]] = a[1]
	}
	return ret, nil
}

func MapToInfoString(data map[string]string) (string, error) {
	var ret = ""
	for k, v := range data {
		ret = fmt.Sprintf("%s\\%s\\%s", ret, k, v)
	}
	return ret, nil
}

func getInfo(server server_info) error {
	return sendUdpMessage(server.ip, []byte(fmt.Sprintf("getinfo %s", server.challenge)))
}

func heartbeat(address *net.UDPAddr, data []string) error {
	for index, v := range server_list {
		if v.ip.String() == address.String() { // already have that server in list
			server_list[index].lastHeartbeat = time.Now()
			server_list[index].status = 1
			return getInfo(server_list[index])
		}
	}
	// Don't found any
	challenge := rand.String(10)
	info := server_info{ip: address, lastHeartbeat: time.Now(), challenge: challenge, status: 1}
	server_list = append(server_list, info)
	return getInfo(server_list[len(server_list)-1])
}

func getserversResponse(ip *net.UDPAddr) error {
	//pack info [byte]x4+[byte]x2
	var output = make([]byte, 0, 1024)
	output = append(output[:], []byte("getserversResponse")...)
	output = append(output[:], []byte("\\")...)
	for _, v := range server_list {
		if v.status == 2 {
			t := make([]byte, 6)
			p := v.ip.IP.To4()
			t[0] = p[0]
			t[1] = p[1]
			t[2] = p[2]
			t[3] = p[3]
			t[4] = byte((v.ip.Port >> 8))
			t[5] = byte(v.ip.Port)
			output = append(output[:], t[:]...)
			output = append(output[:], []byte("\\")...)
		}
	}
	return sendUdpMessage(ip, output)
}

func getServers(address *net.UDPAddr, data []string) error {
	return getserversResponse(address)
}

func infoResponse(address *net.UDPAddr, data []string) error {
	var err error
	infostring, err := infoStringToMap(data[1])
	if err != nil {
		channel_event <- event{t: EVENT_LOG_WARNING, payload: fmt.Sprintf("MAX_INFOSTRING limit exceeded. Dropping request")}
		return nil
	}
	for index, info := range server_list {
		if info.ip.IP.Equal(address.IP) && info.status == 1 {
			server_list[index].clients, err = strconv.Atoi(infostring["clients"])
			server_list[index].fdisable, err = strconv.Atoi(infostring["fdisable"])
			server_list[index].gametype, err = strconv.Atoi(infostring["gametype"])
			server_list[index].hostname = infostring["hostname"]
			server_list[index].mapname = infostring["mapname"]
			server_list[index].protocol, err = strconv.Atoi(infostring["protocol"])
			server_list[index].sv_maxclients, err = strconv.Atoi(infostring["sv_maxclients"])
			server_list[index].truejedi, err = strconv.Atoi(infostring["truejedi"])
			server_list[index].status = 2
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func rateLimiter(address *net.UDPAddr) error { // leaky bukkit implementation
	return nil
}

func processUdpPackets() {
	buffer := make([]byte, MSG_MAXLEN)
	for {
		select {
		case event := <-channel_udp:
			switch event.t {
			case EVENT_ERROR: //error
			case EVENT_SHUTDOWN: //shutdown ( from console )
				return
			default:
			}
		default:
		}
		length, address, err := socket.ReadFromUDP(buffer)
		if err != nil {
			channel_event <- event{t: EVENT_ERROR, err: err, sender: channel_udp}
		}
		raw := string(buffer[4:length])
		length = length - 4
		var data []string
		inx := strings.LastIndex(raw, "\n")
		if inx == length-1 {
			raw = raw[:length-1]
		}
		if strings.Contains(raw, "\n") {
			data = strings.Split(raw, "\n")
		} else {
			data = strings.Split(raw, " ")
		}
		switch data[0] {
		case "heartbeat":
			channel_listrequest <- list_request{t: 0, addr: address, payload: data}
		case "getservers":
			channel_listrequest <- list_request{t: 2, addr: address, payload: data}
		case "infoResponse":
			channel_listrequest <- list_request{t: 1, addr: address, payload: data}
		default:
			channel_event <- event{t: EVENT_LOG_INFO, payload: fmt.Sprintf("Unknown packet received from %s (%s)\n", address.String(), raw)}
		}
	}
}

func checkTimeout() {
	temp := server_list[:0]
	for _, info := range server_list {
		delta := time.Since(info.lastHeartbeat)
		if delta.Milliseconds() <= SERVER_TIMEOUT {
			temp = append(temp, info) // https://github.com/golang/go/wiki/SliceTricks#filtering-without-allocating
		}
	}
	server_list = temp
}

func processCmd() {
	reader := bufio.NewReader(os.Stdin)
	for {
		select {
		case event := <-channel_command:
			switch event.t {
			case EVENT_ERROR: //error
			case EVENT_SHUTDOWN: //shutdown ( from console )
				return
			default:
			}
		default:
		}
		text, err := reader.ReadString('\n')
		if err != nil {
			channel_event <- event{t: EVENT_ERROR, err: err, sender: channel_command}
		}
		fmt.Println("Received: " + text + "\n") // UNQLSS: TODO
	}
}

func serverListWorker() {
	timeout_ticker := time.Tick(SERVER_TIMEOUT * time.Second)
	var err error
	for {
		select {
		case <-timeout_ticker:
			{
				checkTimeout()
			}
		case request := <-channel_listrequest:
			{
				switch request.t {
				case REQUEST_HEARTBEAT:
					err = heartbeat(request.addr, request.payload)
				case REQUEST_INFORESPONSE:
					err = infoResponse(request.addr, request.payload)
				case REQUEST_GETSERVERS:
					err = getServers(request.addr, request.payload)
				default:
					channel_event <- event{t: 3, payload: fmt.Sprintf("Received unknown request ( %d )", request.t)}
				}
			}
		case event := <-channel_serverlist:
			{
				switch event.t {
				case EVENT_ERROR: //error
				case EVENT_SHUTDOWN: //shutdown ( from console )
					return
				default:
				}
			}
		}
		if err != nil {
			channel_event <- event{t: 0, err: err}
			return
		}
	}
}

func initUdpSocket(address string, port int) error {
	var err error
	var addr *net.UDPAddr
	addr, err = net.ResolveUDPAddr("udp", address+":"+strconv.Itoa(int(port)))
	if err != nil {
		channel_event <- event{t: EVENT_ERROR, err: err, sender: channel_udp}
		return err
	}
	socket, err = net.ListenUDP("udp", addr)
	if err != nil {
		channel_event <- event{t: EVENT_ERROR, err: err, sender: channel_udp}
		return err
	}
	return nil
}

func shutdownUdpSocket() error {
	var err error
	err = socket.Close()
	return err
}

func StartDeamon(port int) error {
	var err error
	var logger *logrus.Logger
	var running bool

	logger, err = logging.CreateLogger("log")
	if err != nil {
		fmt.Print(err)
	}

	err = initUdpSocket("", port)
	if err != nil {
		logger.Printf("Faied to init udp socket: %s", err)
		return err
	}
	go processUdpPackets()
	go processCmd()
	go serverListWorker()
	running = true
	logger.Printf("Running UDP Server on port %d\n", port)
	for running {
		select {
		case event := <-channel_event:
			switch event.t {
			case EVENT_ERROR: //error
				logger.Error(err)
			case EVENT_SHUTDOWN: //shutdown ( from console )
				logger.Info("Shutdowning server...\n")
				running = false
				if event.sender == channel_command {
					channel_serverlist <- event
					channel_udp <- event
				}
				if event.sender == channel_udp {
					channel_serverlist <- event
					channel_command <- event
				}
				if event.sender == channel_serverlist {
					channel_command <- event
					channel_udp <- event
				}
			case EVENT_LOG_INFO:
				logger.Info(event.payload)
			case EVENT_LOG_WARNING:
				logger.Warning(event.payload)
			}
		}
	}
	err = shutdownUdpSocket()
	if err != nil {
		logger.Printf("Faied to shutdown udp socket: %s", err)
		return err
	}
	return nil
}
