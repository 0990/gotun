package crypto

import (
	"bytes"
	"fmt"
	"github.com/0990/gotun/util"
	"reflect"
	"testing"
)

func Test_EncryptConn(t *testing.T) {
	ahead, err := util.CreateAesGcmAead(util.StringToAesKey("abcd", 32))
	if err != nil {
		t.Fatal(err)
	}

	data := []byte{1, 2, 3}

	var rw = &bytes.Buffer{}

	c, err := NewReaderWriter(rw, GCM, ahead)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 100)
	n, _ := c.Read(buf)

	equal := reflect.DeepEqual(data, buf[0:n])
	if !equal {
		t.Fatal("not equal")
	}
	fmt.Println(buf[0:n])
}

func Test_UnencryptConn(t *testing.T) {
	data := []byte{1, 2, 3}

	var b = bytes.Buffer{}

	c, err := NewReaderWriter(&b, None, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 100)
	n, _ := c.Read(buf)

	fmt.Println(buf[0:n])
	equal := reflect.DeepEqual(data, buf[0:n])
	if !equal {
		t.Fatal("not equal")
	}
}
