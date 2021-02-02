package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	TESTURL   = "https://universal.bigbuckbunny.workers.dev/Consti10/LiveVideo10ms/master/Screenshots/device2.png?xprotocol=https&xhost=raw.githubusercontent.com"
	TIMEOUT   = 10 * time.Second
	PARALLELS = 100
)

func init() {
	log.SetFlags(log.Ltime | log.Lshortfile)
}

func main() {
	var (
		feedIPCh = make(chan string)
	)

	go prepareIPs("ip.txt", feedIPCh)

	downloadTask(feedIPCh, PARALLELS)
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

func downloadTask(feedIPCh <-chan string, parallels int) {
	var (
		downloadCh = make(chan string, parallels)

		count int32 = 0
		wait        = &sync.WaitGroup{}
	)

	go func() {
		for url := range feedIPCh {
			downloadCh <- url
		}

		close(downloadCh)
	}()

	for {
		if atomic.LoadInt32(&count) < int32(parallels) {
			ip, ok := <-downloadCh
			if ok {
				atomic.AddInt32(&count, 1)
				wait.Add(1)

				go func() {
					speed := download(TESTURL, ip, TIMEOUT)

					log.Println(fmt.Sprintf("%15s | %12.1fKB/s", ip, speed))

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
