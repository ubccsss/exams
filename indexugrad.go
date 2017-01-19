package main

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"

	archive "github.com/d4l3k/go-internetarchive"
	"github.com/hypersleep/easyssh"
	"github.com/urfave/cli"
)

var pathRegexp = regexp.MustCompile("^/home/c/(cs\\w+)/public_html/(.*)$")

func indexUGrad(c *cli.Context) error {
	ssh := &easyssh.MakeConfig{
		User:   c.String("user"),
		Server: c.String("server"),
		// Optional key or Password without either we try to contact your agent SOCKET
		Key:  "/.ssh/id_rsa",
		Port: "22",
	}

	// Call Run method with command you want to run on remote server.
	response, err := ssh.Run(`find /home/c/cs*/public_html -name "*.html" -maxdepth 2`)
	// Handle errors
	if err != nil {
		return err
	}
	urls := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for url := range urls {
				resp, err := archive.Snapshot(url)
				if err != nil {
					log.Println(err)
					break
				}
				log.Println(resp)
			}
		}()
	}
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		matches := pathRegexp.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		urls <- fmt.Sprintf("https://www.ugrad.cs.ubc.ca/~%s/%s", matches[1], matches[2])
	}
	close(urls)
	wg.Wait()
	return nil
}
