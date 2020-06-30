package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	pb_tencent "tencent"

	concurrentMap "github.com/fanliao/go-concurrentMap"

	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
)

type RConfig struct {
	UpstreamAddr1       string
	DealidList1         []string
	UpstreamAddr2       string
	DealidList2         []string
	UpstreamAddr3       string
	DealidList3         []string
	UpstreamAddr4       string
	DealidList4         []string
	UpstreamAddr5       string
	DealidList5         []string
	DefaultUpstreamAddr string
	ListenPort          string
	TimesBackToSource   int
	NoExt               int
}

type handle struct {
	addrs []string
}

type bodyContent struct {
	body string
	cnt  int
}

//使用分段map，将锁结构细粒化
var bodyMap = concurrentMap.NewConcurrentMap()

//var bodyMap map[string]bodyContent
var rconfig RConfig

var mutex *sync.Mutex = new(sync.Mutex)

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
			if strings.Contains(e, a) || strings.Contains(a, e) {
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
		mutex.Lock()
		bodyMap.Put(dealid, bodyContent{adid, 0})
		mutex.Unlock()
		*newResponse.GetSeatbid()[0].GetBid()[0].Ext = "ssp" + adid
	} else {
		mutex.Lock()
		bodyMap.Put(dealid, bodyContent{"0", 1})
		mutex.Unlock()
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

	//process request change
	b = bytes.Replace(b, []byte("server"), []byte("schmerver"), -1)
	newRequest := &pb_tencent.Request{}
	err = proto.Unmarshal(b, newRequest)

	addr := rconfig.DefaultUpstreamAddr
	//if newRequest.Device.DeviceId != nil {

	if contains(rconfig.DealidList1, newRequest.Impression[0].GetDealid(), false) {
		addr = rconfig.UpstreamAddr1
	} else if contains(rconfig.DealidList2, newRequest.Impression[0].GetDealid(), false) {
		addr = rconfig.UpstreamAddr2
	} else if contains(rconfig.DealidList3, newRequest.Impression[0].GetDealid(), false) {
		addr = rconfig.UpstreamAddr3
	} else if contains(rconfig.DealidList4, newRequest.Impression[0].GetDealid(), false) {
		addr = rconfig.UpstreamAddr4
	} else if contains(rconfig.DealidList5, newRequest.Impression[0].GetDealid(), false) {
		addr = rconfig.UpstreamAddr5
	}
	//}
	fmt.Println(newRequest.Impression[0].GetDealid() + " ==>" + addr)
	remote, err := url.Parse("http://" + addr)
	if err != nil {
		panic(err)
	}
	// if  in bodyMap,return body directly
	dealid := newRequest.Impression[0].GetDealid()
	mutex.Lock()
	bodycontent, ok := bodyMap.Get(dealid)
	mutex.Unlock()
	if ok == nil {
		if rand.Intn(rconfig.TimesBackToSource) > 1 {
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

func startServer() {
	//被代理的服务器host和port
	h := &handle{}
	err := http.ListenAndServe(":"+rconfig.ListenPort, h)
	if err != nil {
		log.Fatalln("ListenAndServe: ", err)
	}
}

func main() {

	//检测超过100ms的锁
	//syncT.Opts.DeadlockTimeout = time.Millisecond * 100

	viper.SetConfigName("tencentconfig")
	viper.AddConfigPath(".")
	//bodyMap = make(map[string]bodyContent)
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
	viper.Unmarshal(&rconfig)
	fmt.Println(rconfig)

	startServer()
}
