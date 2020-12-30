package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	fastHttp "github.com/valyala/fasthttp"
	"log"
	"os"
	"strings"
	"sync"
	"tencentgo/model"
	pb_tencent "tencentgo/model/tencent"
	"time"
)

var mutex sync.Mutex

type RConfig struct {
	OpenLogs            bool
	LogPath             string
	UpstreamAddrs       []string
	DefaultUpstreamAddr string
	ListenPort          string
	TimesBackToSource   int
	NoExt               int
}

type upStreamStruct struct {
	ipAddr string
	deals  []string
}

type handle struct {
	addrs []string
}

type bodyContent struct {
	body string
	cnt  int
}

var (
	bodyMap = &sync.Map{}

	configMap map[string]upStreamStruct

	allDealsMap   map[string]bool //存放所有deals
	rconfig       RConfig
	client        = NewFastHttpClient()
	err           error
	upStreamCount int = 0
)

func NewFastHttpClient() *fastHttp.Client {
	return &fastHttp.Client{
		MaxConnsPerHost:    512000,
		MaxConnWaitTimeout: 10 * time.Second,
		//ReadTimeout:         8 * time.Second,
		//WriteTimeout:        8 * time.Second,
		//MaxIdleConnDuration: 8 * time.Second,
	}
}

func FastHttpRoutrip(ctx *fastHttp.RequestCtx) *fastHttp.RequestCtx {

	req := &ctx.Request
	resp := &ctx.Response

	//process request change
	b := bytes.Replace(req.Body(), []byte("server"), []byte("schmerver"), -1)
	newRequest := &pb_tencent.Request{}
	err := proto.Unmarshal(b, newRequest)

	if err != nil {
		return nil
	}

	body, err := proto.Marshal(newRequest)
	if err != nil {
		log.Println(err)
		return nil
	}

	req.SetBody(body)

	//client := &fastHttp.Client{}
	if err := client.DoTimeout(req, resp, 5*time.Second); err != nil {
		log.Println("fasthttp do err is :", err)
		resp.SetStatusCode(204)
		return nil
	}

	//response handler
	b = bytes.Replace(resp.Body(), []byte("server"), []byte("schmerver"), -1)
	newResponse := &pb_tencent.Response{}

	err = proto.Unmarshal(b, newResponse)
	dealid := newRequest.Impression[0].GetDealid()
	if len(newResponse.GetSeatbid()) > 0 && len(newResponse.GetSeatbid()[0].GetBid()) > 0 {
		adid := newResponse.Seatbid[0].Bid[0].GetAdid()
		bodyMap.Store(dealid, bodyContent{adid, 0})
	} else {
		bodyMap.Store(dealid, bodyContent{"0", 1})
	}

	fmt.Println("roundTrip REQREQREQREQ      " + newRequest.String())
	fmt.Println("roundTrip RESPRESPRESPRESP  " + newResponse.String())

	// pb object to response body and return to hhtp
	data, err := proto.Marshal(newResponse) //TODO: if no changed, just send original pb to http
	resp.SetBody(data)
	resp.Header.SetContentLength(len(data))
	return ctx
}

func (this *handle) ServeHTTP(ctx *fastHttp.RequestCtx) {
	b := ctx.Request.Body()

	//process request change
	b = bytes.Replace(b, []byte("server"), []byte("schmerver"), -1)
	newRequest := &pb_tencent.Request{}
	err := proto.Unmarshal(b, newRequest)

	if err != nil {
		log.Println("proto parse err is :", err)
		return
	}

	//记录落盘数据
	DataReport(newRequest)

	addr := rconfig.DefaultUpstreamAddr

	var newRequestDealId string
	if len(newRequest.Impression) > 0 {
		newRequestDealId = newRequest.Impression[0].GetDealid()

		if configMap == nil {
			panic("config map is nil")
		}

		for _, conV := range configMap {
			for _, vDs := range conV.deals {
				if strings.Contains(vDs, newRequestDealId) || strings.Contains(newRequestDealId, vDs) {
					addr = conV.ipAddr
					break
				}
			}
		}

		if err != nil {
			panic(err)
		}

		bodycontent, ok := bodyMap.Load(newRequestDealId)

		_, dealOk := allDealsMap[newRequestDealId] //判断此dealId 是否在配置文件deal列表中
		if ok && dealOk && bodycontent != nil {
			if upStreamCount <= rconfig.TimesBackToSource {
				//fmt.Println(newRequestDealId + " ==>" + addr)
				id := newRequest.GetId()
				bidid := newRequest.Impression[0].GetId()
				adid := bodycontent.(bodyContent).body
				price := float32(9000)
				extid := "ssp" + adid

				err = proto.Unmarshal(b, newRequest)
				newResponse := &pb_tencent.Response{}
				if adid != "0" {
					newResponse = &pb_tencent.Response{
						Id: &id,
						Seatbid: []*pb_tencent.Response_SeatBid{
							{
								Bid: []*pb_tencent.Response_Bid{
									{
										Id:    &bidid,
										Impid: &bidid,
										Price: &price,
										Adid:  &adid,
										Ext:   &extid},
								},
							},
						},
					}
				} else {
					newResponse = &pb_tencent.Response{
						Id: &id,
					}
				}
				data, err := proto.Marshal(newResponse) //TODO: if no changed, just send original pb to http
				if err != nil {
					ctx.Response.SetStatusCode(204)
				}
				ctx.Write(data)

				fmt.Println("serverHttp REQREQREQREQ         " + newRequest.String())
				fmt.Println("serverHttp RESPRESPRESPRESP     " + newResponse.String())
				upStreamCount++
				return
			} else {
				upStreamCount = 0
			}
		}

		ctx.SetBody(b)
		ctx.Request.SetRequestURI("http://" + addr + "/tencent.htm")
		ctx.Request.Header.Set("Content-Type", "application/x-protobuf;charset=UTF-8")

		if fCtx := FastHttpRoutrip(ctx); fCtx == nil {
			log.Println("ProxyPoolHandler got an error: ", err)
			ctx.SetStatusCode(204)
			return
		}

	}
}

//收集数据
func DataReport(bidRequest *pb_tencent.Request) {

	setAdxContext := func(deviceId, deviceType, uaStr, bidTime, ipaddress string, adxc *model.AdxContext) {
		adxc.AdxVisitorId = deviceId
		adxc.DeviceType = deviceType
		adxc.IpAddress = ipaddress
		adxc.UserAgent = uaStr
		adxc.BidTime = bidTime
	}

	adxc := new(model.AdxContext)
	device := bidRequest.GetDevice()
	timeNow := time.Now()
	bidtime := fmt.Sprintf("%02d%02d%02d%02d%02d%02d", timeNow.Year(), timeNow.Month(), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second())

	if device != nil {
		if device.GetIdfa() != "" {
			setAdxContext(device.GetIdfa(), model.IDFA, device.GetUa(), bidtime, device.GetIp(), adxc)
		} else if device.GetImei() != "" {
			setAdxContext(device.GetImei(), model.IMEI, device.GetUa(), bidtime, device.GetIp(), adxc)
		} else if device.GetOpenudid() != "" {
			setAdxContext(device.GetOpenudid(), model.OPENUUID, device.GetUa(), bidtime, device.GetIp(), adxc)
		} else if device.GetAndroidid() != "" {
			setAdxContext(device.GetAndroidid(), model.ANDROIDID, device.GetUa(), bidtime, device.GetIp(), adxc)
		} else if device.GetMac() != "" {
			setAdxContext(device.GetMac(), model.MAC, device.GetUa(), bidtime, device.GetIp(), adxc)
		} else {
			if bidRequest.GetUser() != nil {
				adxc.AdxVisitorId = fmt.Sprintf("%v", bidRequest.GetUser().GetId())
				setAdxContext(bidRequest.GetUser().GetId(), model.PC, device.GetUa(), bidtime, device.GetIp(), adxc)
			}
		}
	}

	dataRes := fmt.Sprintf("%v|%v|%v|%v", adxc.BidTime, adxc.AdxVisitorId, adxc.DeviceType, adxc.UserAgent)
	WriteToFile(rconfig.OpenLogs, rconfig.LogPath, dataRes)

}

func WriteToFile(open bool, path string, res string) {

	if !open {
		return
	}

	mutex.Lock()
	now := time.Now()

	filePath := fmt.Sprintf("%v/fl_%v_%02d%02d%02d%02d.csv", path, rconfig.ListenPort, now.Year(), now.Month(), now.Day(), now.Hour())
	file, _ := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)

	//及时关闭file句柄
	defer file.Close()
	//写入文件时，使用带缓存的 *Writer
	write := bufio.NewWriter(file)

	//写入文件
	write.WriteString(res + "\n")

	//Flush将缓存的文件真正写入到文件中
	write.Flush()
	mutex.Unlock()
}

func startServer() {
	//被代理的服务器host和port
	h := &handle{}

	err := fastHttp.ListenAndServe(":"+rconfig.ListenPort, h.ServeHTTP)

	if err != nil {
		log.Fatalln("start http err is : ", err)
	}
}

func main() {

	//initProxy() //初始化代理池
	//proxy.SetProduction()
	configMap = make(map[string]upStreamStruct)
	allDealsMap = make(map[string]bool)

	viper.SetConfigName("tencentconfig")
	viper.AddConfigPath(".")
	//bodyMap = make(map[string]bodyContent)
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	err = viper.Unmarshal(&rconfig)
	if err != nil {
		panic(err)
	}

	if len(rconfig.UpstreamAddrs) != 0 {
		for _, v := range rconfig.UpstreamAddrs {

			upStream := strings.Split(v, "|")
			//golang map 遍历输出无序的，所以加入id
			id := upStream[0]
			usSplit := strings.Split(upStream[1], ",")
			deals := usSplit[1:]
			uss := &upStreamStruct{
				ipAddr: usSplit[0],
				deals:  deals,
			}

			configMap[id] = *uss

			for _, cV := range configMap {
				for _, v := range cV.deals {
					allDealsMap[v] = true
				}
			}
		}
	}

	fmt.Println(rconfig)
	fmt.Println(allDealsMap)

	startServer()
}
