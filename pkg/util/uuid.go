package util

import (
	"github.com/google/uuid"
	"math/rand"
)

func NewUUID() string {
	return uuid.New().String()
}

// 生成指定长度的随机字符串
func TraceID(l int) string {
	str := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	bytes := []byte(str)
	result := []byte{}
	//r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < l; i++ {
		result = append(result, bytes[rand.Intn(len(bytes))])
	}
	return string(result)
}
