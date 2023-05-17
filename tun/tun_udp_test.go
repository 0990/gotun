package tun

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"net"
	"testing"
	"time"
)

func Test_UDP(t *testing.T) {
	targetAddr := "127.0.0.1:7007"
	startUDPEchoServer(targetAddr)

	relayAddr := "127.0.0.1:6000"
	s, err := NewServer(Config{
		Name:          "udp",
		Input:         fmt.Sprintf("udp@%s", relayAddr),
		Output:        fmt.Sprintf("udp@%s", targetAddr),
		InDecryptKey:  "",
		InDecryptMode: "",
		OutCryptKey:   "",
		OutCryptMode:  "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()
	time.Sleep(time.Second)
	err = echoUDP(relayAddr)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_UDPTun(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	targetAddr := "127.0.0.1:7007"
	startUDPEchoServer(targetAddr)

	relayClientAddr := "127.0.0.1:6000"
	relayServerAddr := "127.0.0.1:6001"
	c, err := NewServer(Config{
		Name:          "udp_tun_client",
		Input:         fmt.Sprintf("udp@%s", relayClientAddr),
		Output:        fmt.Sprintf("udp@%s", relayServerAddr),
		InDecryptKey:  "",
		InDecryptMode: "",
		OutCryptKey:   "111111",
		OutCryptMode:  "gcm",
	})
	if err != nil {
		t.Fatal(err)
	}

	c.Run()

	s, err := NewServer(Config{
		Name:          "udp_tun_server",
		Input:         fmt.Sprintf("udp@%s", relayServerAddr),
		Output:        fmt.Sprintf("udp@%s", targetAddr),
		InDecryptKey:  "111111",
		InDecryptMode: "gcm",
		OutCryptKey:   "",
		OutCryptMode:  "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()
	time.Sleep(time.Second)
	err = echoUDP(relayClientAddr)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_UDPTunTCP(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	targetAddr := "127.0.0.1:7007"
	startUDPEchoServer(targetAddr)

	relayClientAddr := "127.0.0.1:6000"
	relayServerAddr := "127.0.0.1:6001"
	c, err := NewServer(Config{
		Name:          "udp_tun_client",
		Input:         fmt.Sprintf("udp@%s", relayClientAddr),
		Output:        fmt.Sprintf("tcp@%s", relayServerAddr),
		InDecryptKey:  "",
		InDecryptMode: "",
		OutCryptKey:   "111111",
		OutCryptMode:  "gcm",
	})
	if err != nil {
		t.Fatal(err)
	}

	c.Run()

	s, err := NewServer(Config{
		Name:          "udp_tun_server",
		Input:         fmt.Sprintf("tcp@%s", relayServerAddr),
		Output:        fmt.Sprintf("udp@%s", targetAddr),
		InDecryptKey:  "111111",
		InDecryptMode: "gcm",
		OutCryptKey:   "",
		OutCryptMode:  "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()
	time.Sleep(time.Second)
	err = echoUDP(relayClientAddr)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("over")
}

func startUDPEchoServer(targetAddr string) error {
	addr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		return err
	}
	listen, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	go func() {
		for {
			var data [1024]byte
			n, addr, err := listen.ReadFromUDP(data[:])
			if err != nil {
				fmt.Println(err)
				break
			}

			fmt.Printf("echoserver receive,addr:%v data:%v count:%v\n", addr, string(data[:n]), n)
			_, err = listen.WriteToUDP(data[:n], addr)
			if err != nil {
				fmt.Println(err)
				continue
			}
		}
	}()
	return nil
}

func echoUDP(clientAddr string) error {
	addr, err := net.ResolveUDPAddr("udp", clientAddr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}
	before := time.Now()

	err = checkEchoReplyUDP(conn)
	if err != nil {
		return err
	}
	elapse := time.Since(before).Milliseconds()
	fmt.Println("elapse:", elapse)
	return nil
}

func checkEchoReplyUDP(conn net.Conn) error {

	send1 := "hello"
	_, err := conn.Write([]byte(send1))
	if err != nil {
		return err
	}

	send2 := "world"
	_, err = conn.Write([]byte(send2))
	if err != nil {
		return err
	}

	//conn.SetReadDeadline(time.Now().Add(time.Second * 2))
	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}

	ret1 := string(buf[0:n])

	if ret1 != send1 {
		return fmt.Errorf("echo send:%s receive:%s", send1, buf[0:n])
	}

	n, err = conn.Read(buf)
	if err != nil {
		return err
	}

	ret2 := string(buf[0:n])

	if send2 != ret2 {
		return fmt.Errorf("echo send:%s receive:%s", send2, ret2)
	}
	return nil
}
