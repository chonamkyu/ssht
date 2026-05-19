package multi

import (
	"sync"

	"github.com/chonamkyu/ssht/internal/config"
	sshclient "github.com/chonamkyu/ssht/internal/ssh"
)

type Result struct {
	Host   string `json:"host"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

func Execute(hosts []config.Host, command string) []Result {
	results := make([]Result, len(hosts))
	var wg sync.WaitGroup

	for i, host := range hosts {
		wg.Add(1)
		go func(idx int, h config.Host) {
			defer wg.Done()

			client, err := sshclient.Connect(&h)
			if err != nil {
				results[idx] = Result{Host: h.Name, Error: err.Error()}
				return
			}
			defer client.Close()

			output, err := client.Execute(command)
			if err != nil {
				results[idx] = Result{Host: h.Name, Output: output, Error: err.Error()}
				return
			}

			results[idx] = Result{Host: h.Name, Output: output}
		}(i, host)
	}

	wg.Wait()
	return results
}
