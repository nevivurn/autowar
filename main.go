package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gosuri/uilive"
)

var (
	timeout = flag.Duration("timeout", time.Second, "connection timeout")
	cookie  = flag.String("cookie", "", "access token")
	period  = flag.Duration("period", 75*time.Millisecond, "click period")
	target  = flag.String("target", "301", "which building")

	rate, count int32

	bypass, logger *log.Logger
	client         *http.Client
)

// Endpoints
const (
	endBase      = "https://api.snuwar.io:8080"
	endLike      = endBase + "/like"
	endHandshake = endBase + "/locationRanking/"
)

type request struct {
	D        int    `json:"d"`
	Location string `json:"location"`
}

type response struct {
	MasterInfo struct {
		Nickname, OccupyWord string
		ClickCount           int
	}
	Likes struct {
		A, B, C int
		E       int
	}
}

type liker struct {
	loc string
	e   int
}

func nextCode(e int) int {
	return (e ^ 34250128) + 532
}

func (l *liker) handshake() error {
	req, err := http.NewRequest(http.MethodGet, endHandshake+l.loc, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Cookie", "auth="+*cookie)

	rsp, err := client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(ioutil.Discard, rsp.Body)
	rsp.Body.Close()
	return nil
}

func (l *liker) like() error {
	data, _ := json.Marshal(request{D: nextCode(l.e), Location: l.loc})
	req, err := http.NewRequest(http.MethodPost, endLike, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Cookie", "auth="+*cookie)

	rsp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		bypass.Printf("status: " + rsp.Status)
		return nil
	}

	var body response
	if err := json.NewDecoder(rsp.Body).Decode(&body); err != nil {
		bypass.Printf("%v", err)
		return nil
	}
	atomic.AddInt32(&count, 1)

	pct := float32(body.MasterInfo.ClickCount) / float32(body.Likes.A+body.Likes.B+body.Likes.C)

	logger.Printf("%s: Red: %d, Blue: %d, Green: %d (rate: %.1f) (%s: %f%%)",
		l.loc, body.Likes.A, body.Likes.B, body.Likes.C,
		float64(atomic.LoadInt32(&rate))/5,
		body.MasterInfo.Nickname, pct*100)

	l.e = body.Likes.E
	return nil
}

func main() {
	flag.Parse()

	if *cookie == "" {
		log.Fatal("specify a token please")
	}

	client = &http.Client{Timeout: *timeout}

	w := uilive.New()
	bypass = log.New(w.Bypass(), "error: ", log.LstdFlags)
	logger = log.New(w, "", log.LstdFlags)
	w.Start()

	l := liker{loc: *target}
	if err := l.handshake(); err != nil {
		bypass.Println(err)
	}

	go func() {
		tick := time.Tick(5 * time.Second)
		for {
			<-tick
			atomic.StoreInt32(&rate, atomic.SwapInt32(&count, 0))
		}

	}()

	for {
		if err := l.like(); err != nil {
			bypass.Println(err)
			if err := l.handshake(); err != nil {
				bypass.Println(err)
			}
		}
		time.Sleep(*period)
	}
}
