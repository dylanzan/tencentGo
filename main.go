package main

import (
	"bytes"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	pb_tencent "tencentgo/model/tencent"
)

type RConfig struct {
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

	allDealsMap map[string]bool //存放所有deals
	rconfig     RConfig
)

//var bodyMap map[string]bodyContent

//var mutex = new(sync.Mutex)

type transport struct {
	http.RoundTripper
}

var _ http.RoundTripper = &transport{}

func contains(s []string, e string, isExact bool) bool {
	for _, a := range s {
		a = strings.TrimSpace(a)
		if isExact {
			if a == e {
				return true
			}
		} else {
			if strings.Contains(a, e) || strings.Contains(e, a) {
				return true
			}
		}
	}
	return false
}

func (t *transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	// copy request
	b, err := ioutil.ReadAll(req.Body)

	//process request change
	b = bytes.Replace(b, []byte("server"), []byte("schmerver"), -1)
	newRequest := &pb_tencent.Request{}
	err = proto.Unmarshal(b, newRequest)

	//modify b if necessary

	//turn to pb and set back
	data, err := proto.Marshal(newRequest) //TODO: if no changed, just send original pb to http
	body := ioutil.NopCloser(bytes.NewReader(data))
	req.Body = body
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Length", strconv.Itoa(len(data)))

	//set back
	//req.Body = ioutil.NopCloser(bytes.NewBuffer(b))

	// reverse proxy
	resp, err = t.RoundTripper.RoundTrip(req)

	// error to nil return
	if err != nil {
		return nil, err
	}
	err = req.Body.Close()
	if err != nil {
		return nil, err
	}

	//TODO: should be error return here, to find out a new solution

	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	err = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	b = bytes.Replace(b, []byte("server"), []byte("schmerver"), -1)
	//body = ioutil.NopCloser(bytes.NewReader(b))
	newResponse := &pb_tencent.Response{}

	err = proto.Unmarshal(b, newResponse)
	dealid := newRequest.Impression[0].GetDealid()
	if len(newResponse.GetSeatbid()) > 0 && len(newResponse.GetSeatbid()[0].GetBid()) > 0 {
		adid := newResponse.Seatbid[0].Bid[0].GetAdid()
		//mutex.Lock()
		bodyMap.Store(dealid, bodyContent{adid, 0})
		//mutex.Unlock()
		*newResponse.GetSeatbid()[0].GetBid()[0].Ext = "ssp" + adid
	} else {
		//mutex.Lock()
		bodyMap.Store(dealid, bodyContent{"0", 1})
		//mutex.Unlock()
	}

	fmt.Println("REQREQREQREQ\n" + newRequest.String())
	fmt.Println("RESPRESPRESPRESP\n" + newResponse.String())

	// pb object to response body and return to hhtp
	data, err = proto.Marshal(newResponse) //TODO: if no changed, just send original pb to http
	body = ioutil.NopCloser(bytes.NewReader(data))
	resp.Body = body
	resp.ContentLength = int64(len(data))
	resp.Header.Set("Content-Length", strconv.Itoa(len(data)))
	return resp, nil
}

func (this *handle) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	b, err := ioutil.ReadAll(r.Body)

	if err != nil {
		log.Printf("read all err is %v", err)
	}

	//process request change
	b = bytes.Replace(b, []byte("server"), []byte("schmerver"), -1)
	newRequest := &pb_tencent.Request{}
	err = proto.Unmarshal(b, newRequest)

	addr := rconfig.DefaultUpstreamAddr
	//if newRequest.Device.DeviceId != nil {

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
				}
			}
		}

		remote, err := url.Parse("http://" + addr)
		if err != nil {
			panic(err)
		}
		// if  in bodyMap,return body directly
		//	dealid := newRequest.Impression[0].GetDealid()
		//mutex.Lock()
		bodycontent, ok := bodyMap.Load(newRequestDealId)
		//mutex.Unlock()

		_, dealOk := allDealsMap[newRequestDealId] //判断此dealId 是否在配置文件deal列表中
		if ok && dealOk && bodycontent != nil {
			//rand.Seed(time.Now().UnixNano())
			if rand.Intn(rconfig.TimesBackToSource) > 1 {
				fmt.Println(newRequestDealId + " ==>" + addr)
				id := newRequest.GetId()
				bidid := newRequest.Impression[0].GetId()
				adid := bodycontent.(bodyContent).body
				price := float32(9000)
				extid := "ssp" + adid
				//mutex.Lock()
				//bodyMap[dealid] = bodyContent{adid, bodycontent.cnt + 1}
				//mutex.Unlock()
				err = proto.Unmarshal(b, newRequest)
				newResponse := &pb_tencent.Response{}
				if adid != "0" {
					newResponse = &pb_tencent.Response{
						Id: &id,
						Seatbid: []*pb_tencent.Response_SeatBid{
							{
								Bid: []*pb_tencent.Response_Bid{
									{Id: &bidid,
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
					w.WriteHeader(204)
				}
				//bodyMap[*(newRequest.Impression[0].Dealid)] = bodyContent{bodycontent.body, bodycontent.cnt + 1}
				w.Write(data)

				fmt.Println("REQREQREQREQ\n" + newRequest.String())
				fmt.Println("RESPRESPRESPRESP\n" + newResponse.String())
				return
			}
		}

		//if not in bodyMap, reverseProxy and transpot RoundTrip,
		body := ioutil.NopCloser(bytes.NewReader(b))
		r.Body = body
		proxy := httputil.NewSingleHostReverseProxy(remote)
		proxy.Transport = &transport{http.DefaultTransport}
		proxy.ServeHTTP(w, r)
	}

}

func startServer() {
	//被代理的服务器host和port
	h := &handle{}

	srv := http.Server{
		Addr:    ":" + rconfig.ListenPort,
		Handler: h,
	}

	fmt.Println(srv.Addr)
	err := srv.ListenAndServe()

	if err != nil {
		log.Fatalln("ListenAndServe: ", err)
	}
}

func main() {

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
