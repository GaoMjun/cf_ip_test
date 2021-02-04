package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	TESTURL   = "https://speed.cloudflare.com/__down?bytes=104857600"
	TIMEOUT   = 10 * time.Second
	PARALLELS = 20
	PINGCOUNT = 10
	IPPATH    = "ip.txt"
)

func init() {
	log.SetFlags(log.Ltime | log.Lshortfile)
}

type result struct {
	ip                       string
	bandwidth, loss, latency float64
}

func main() {
	var (
		feedIPCh = make(chan string)

		ch1 = make(chan interface{}, 10)
		ch2 = make(chan interface{}, 10)

		wait = &sync.WaitGroup{}

		statistics = map[string]*result{}
		resultCh   = make(chan result, PARALLELS)

		endCh = make(chan struct{})
	)

	go prepareIPs(IPPATH, feedIPCh)

	go func() {
		for ip := range feedIPCh {
			go func(t string) { ch1 <- t }(ip)
			go func(t string) { ch2 <- t }(ip)
		}

		ch1 <- nil
		ch2 <- nil
	}()

	go func() {
		for r := range resultCh {
			log.Println(r)

			if _, ok := statistics[r.ip]; !ok {
				statistics[r.ip] = &result{ip: r.ip}
			}

			if r.bandwidth >= 0 {
				statistics[r.ip].bandwidth = r.bandwidth
			}
			if r.loss >= 0 {
				statistics[r.ip].loss = r.loss
			}
			if r.latency >= 0 {
				statistics[r.ip].latency = r.latency
			}
		}

		fmt.Println("ip,bandwidth,loss,latency")
		for k, v := range statistics {
			fmt.Printf("%s,%.0f,%.2f,%.0f\n", k, v.bandwidth, v.loss, v.latency)
		}

		endCh <- struct{}{}
	}()

	wait.Add(1)
	go func() {
		parallelsTask(ch1, func(a ...interface{}) {
			ip := a[0].(string)
			bandwidth := download(TESTURL, ip, TIMEOUT)
			// log.Println(fmt.Sprintf("%15s | %12.1fKB/s", ip, bandwidth))

			resultCh <- result{ip: ip, bandwidth: bandwidth, loss: -1, latency: -1}
		}, PARALLELS)

		wait.Done()
	}()

	wait.Add(1)
	go func() {
		parallelsTask(ch2, func(a ...interface{}) {
			ip := a[0].(string)
			loss, latency := ping(ip, PINGCOUNT)
			// log.Println(ip, loss, latency)

			resultCh <- result{ip: ip, bandwidth: -1, loss: loss, latency: latency}
		}, PARALLELS)

		wait.Done()
	}()

	wait.Wait()

	close(resultCh)

	<-endCh
}

func prepareIPs(p string, feedIPCh chan<- string) {
	var (
		e error
		f *os.File
		r *bufio.Reader
		l []byte
	)
	defer func() {
		close(feedIPCh)

		if e != nil {
			log.Println(e)
		}
	}()

	if f, e = os.Open(p); e != nil {
		return
	}
	defer f.Close()

	r = bufio.NewReader(f)

	for {
		if l, _, e = r.ReadLine(); e != nil {
			if e == io.EOF {
				e = nil
			}
			return
		}

		line := string(l)

		ip := fmt.Sprintf("%s%d", line, rand.Intn(256))

		feedIPCh <- ip
	}
}

func parallelsTask(ch <-chan interface{}, f func(...interface{}), parallels int) {
	var (
		processCh = make(chan interface{}, parallels)

		count int32 = 0
		wait        = &sync.WaitGroup{}
	)

	go func() {
		for t := range ch {
			if t == nil {
				close(processCh)
			} else {
				processCh <- t
			}
		}
	}()

	for {
		if atomic.LoadInt32(&count) < int32(parallels) {
			t, ok := <-processCh
			if ok {
				atomic.AddInt32(&count, 1)
				wait.Add(1)

				go func() {

					f(t)

					atomic.AddInt32(&count, -1)
					wait.Done()
				}()
			} else {
				break
			}
		} else {
			time.Sleep(time.Millisecond * 1)
		}
	}

	wait.Wait()
}

func download(url, ip string, timeout time.Duration) (speed float64) {
	var (
		err error

		duration      = float64(timeout / time.Second)
		total         = 0
		req           *http.Request
		resp          *http.Response
		underlinkConn net.Conn
		httpClient    = &http.Client{
			Transport: &http.Transport{
				MaxConnsPerHost: 1,
				Dial: func(network, addr string) (c net.Conn, e error) {
					hostport := strings.Split(addr, ":")

					c, e = net.Dial(network, fmt.Sprintf("%s:%s", ip, hostport[1]))
					underlinkConn = c
					return c, e
				},
			},
		}
	)
	defer func() {
		if err != nil {
			// log.Println(err)
		}
	}()

	if req, err = newRequest(url); err != nil {
		return
	}

	start := time.Now()
	processCh := make(chan struct{})
	go func() {
		defer func() {
			processCh <- struct{}{}
		}()

		if resp, err = httpClient.Do(req); err != nil {
			return
		}
		defer resp.Body.Close()

		var (
			b = make([]byte, 32*1024)
			n = 0
		)
		for {
			n, err = resp.Body.Read(b)
			total += n
			if err != nil {
				if err == io.EOF {
					err = nil
				}
				return
			}
		}
	}()

	select {
	case <-processCh:
		duration = time.Now().Sub(start).Seconds()
	case <-time.After(timeout):
		if underlinkConn != nil {
			underlinkConn.Close()
		}
	}

	speed = float64(total) / duration

	return
}

func newRequest(i string) (r *http.Request, err error) {
	if r, err = http.NewRequest(http.MethodGet, i, nil); err != nil {
		return
	}

	r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36 Edg/83.0.478.64")
	r.Header.Set("Referer", i)

	return
}

func ping(ip string, count int) (loss, avg float64) {
	var (
		err error

		cmd = exec.Command("ping", "-c", strconv.Itoa(count), ip)
		r   io.ReadCloser
	)
	defer func() {
		if err != nil {
			log.Println(err)
		}
	}()

	loss = 100
	avg = 0

	if r, err = cmd.StdoutPipe(); err != nil {
		return
	}
	defer r.Close()

	go func() {
		var (
			e    error
			bufr = bufio.NewReader(r)
			l    []byte
			c    = -3
		)
		defer func() {
			if e != nil {
				// log.Println(e)
			}
		}()

		for {
			if l, _, e = bufr.ReadLine(); e != nil {
				return
			}

			// log.Println(string(l))
			c += 1

			if c == count+1 {
				if loss, err = parsePingResponsePacketLoss(string(l)); err != nil {
					return
				}
			}
			if c == count+2 {
				if avg, err = parsePingResponseAVG(string(l)); err != nil {
					return
				}
			}
		}
	}()

	if err = cmd.Start(); err != nil {
		return
	}

	if err = cmd.Wait(); err != nil {
		return
	}

	return
}

func parsePingResponsePacketLoss(l string) (f float64, err error) {
	if !strings.Contains(l, "packet loss") {
		f = 100
		err = errors.New("invalid packet loss line")
		return
	}

	l = strings.Split(l, ", ")[2]
	l = strings.Split(l, " ")[0]
	l = l[:len(l)-1]

	if f, err = strconv.ParseFloat(l, 64); err != nil {
		return
	}

	f /= 100

	return
}

func parsePingResponseAVG(l string) (f float64, err error) {
	if !strings.Contains(l, "avg") {
		f = 0
		err = errors.New("invalid round-trip line")
		return
	}

	l = strings.Split(l, "/")[4]

	if f, err = strconv.ParseFloat(l, 64); err != nil {
		return
	}

	return
}
