package tun

import (
	"fmt"
	"github.com/0990/gotun/server/socks5x"
	"github.com/0990/socks5"
	"github.com/sirupsen/logrus"
	"log"
	"testing"
	"time"
)

func Test_Socks5X(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	targetAddr := "127.0.0.1:7007"
	startSocks5X(7007)

	relayClientAddr := "127.0.0.1:6000"
	relayServerAddr := "127.0.0.1:6005"
	s, err := NewServer(Config{
		Name:          "tcp",
		Input:         fmt.Sprintf("tcp@%s", relayServerAddr),
		Output:        fmt.Sprintf("tcp@%s", targetAddr),
		InDecryptKey:  "goodweather",
		InDecryptMode: "gcm",
		OutCryptKey:   "",
		OutCryptMode:  "",
	})
	if err != nil {
		t.Fatal(err)
	}

	s.Run()

	time.Sleep(2 * time.Second)
	c, err := NewServer(Config{
		Name:   "tcp",
		Input:  fmt.Sprintf("socks5x@%s", relayClientAddr),
		Output: fmt.Sprintf("tcp@%s", relayServerAddr),
		//InProtoCfg:    inSocks5xStr(InProtoSocks5X{}),
		InDecryptKey:  "",
		InDecryptMode: "",
		OutCryptKey:   "goodweather",
		OutCryptMode:  "gcm",
		//	OutExtend:     "{\"mux_conn\":10}",
	})
	if err != nil {
		t.Fatal(err)
	}

	c.Run()

	time.Sleep(time.Second * 2)

	socks5ClientTest(socks5.ClientCfg{
		ServerAddr: relayClientAddr,
		UserName:   "",
		Password:   "",
		UDPTimout:  300,
		TCPTimeout: 300,
	}, t)

	time.Sleep(time.Second * 100)
}

func startSocks5X(listenPort int) error {
	s, err := socks5x.NewServer(listenPort, 300, 200)
	if err != nil {
		return err
	}
	err = s.Run()
	if err != nil {
		return err
	}
	return nil
}

func socks5ClientTest(cfg socks5.ClientCfg, t *testing.T) {
	sc := socks5.NewSocks5Client(cfg)
	conn, err := sc.Dial("tcp", "127.0.0.1:9999")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		buffer := make([]byte, 100)
		for {
			//read
			n, err := conn.Read(buffer)
			if err != nil {
				log.Println(err)
				return
			}
			fmt.Println("receive data:", string(buffer[:n]))
		}
	}()

	count := 0
	for {
		count++
		if count < 2 {
			_, err := conn.Write([]byte("hello"))
			if err != nil {
				fmt.Println(err)
			}
		} else {
			fmt.Println("silence")
		}
		if count == 200 {
			_, err := conn.Write([]byte("hello"))
			if err != nil {
				fmt.Println(err)
			}
		}
		fmt.Println(count)
		time.Sleep(time.Second)
	}
}
