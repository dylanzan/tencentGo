module tencentgo

go 1.14

require (
	github.com/fanliao/go-concurrentMap v0.0.0-20141114143905-7d2d7a5ea67b
	github.com/golang/protobuf v1.4.0
	github.com/spf13/viper v1.6.3
	github.com/wxnacy/wgo v1.0.4
	golang.org/x/sys v0.0.0-20191120155948-bd437916bb0e // indirect
	tencent v0.0.0
)

replace tencent => ./src/model/tencent
