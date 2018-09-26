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

const SSHTimeout = 60

func indexUGrad(c *cli.Context) error {
	ssh := &easyssh.MakeConfig{
		User:   c.String("user"),
		Server: c.String("server"),
		// Optional key or Password without either we try to contact your agent SOCKET
		Key:  "/.ssh/id_rsa",
		Port: "22",
	}

	// Call Run method with command you want to run on remote server.
	response, _, _, err := ssh.Run(`find /home/c/cs*/public_html -name "*.html" -maxdepth 2`, SSHTimeout)
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
		if url, ok := ugradPathToHTTP(line); ok {
			urls <- url
		}
	}
	close(urls)
	wg.Wait()
	return nil
}

var pathRegexp = regexp.MustCompile("^/home/c/(cs\\w+)/public_html/(.*)$")

func ugradPathToHTTP(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if len(path) == 0 {
		return "", false
	}
	matches := pathRegexp.FindStringSubmatch(path)
	if len(matches) != 3 {
		return "", false
	}
	return fmt.Sprintf("https://www.ugrad.cs.ubc.ca/~%s/%s", matches[1], matches[2]), true
}
