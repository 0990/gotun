package util

import "time"

// BeijingLocation 固定使用北京时间（UTC+8），不依赖服务器时区或系统时区数据库。
var BeijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// FormatBeijingTime 将时间转换为北京时间后按指定格式展示。
func FormatBeijingTime(t time.Time, layout string) string {
	return t.In(BeijingLocation).Format(layout)
}
