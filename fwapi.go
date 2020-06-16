package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	mcpinger "github.com/Raqbit/mc-pinger"
)

var servers = make(map[string]mcpinger.Pinger)

type Serben struct {
	Online uint `json:"online"`
	Max    uint `json:"max"`
}

func main() {
	jsonServers, err := os.Open("servers.json")
	if err != nil {
		log.Fatal(err)
	}

	bytesServers, err := ioutil.ReadAll(jsonServers)
	jsonServers.Close()
	if err != nil {
		log.Fatal(err)
	}

	var tmp map[string]string
	if err := json.Unmarshal([]byte(bytesServers), &tmp); err != nil {
		log.Fatal(err)
	}

	for k, v := range tmp {
		s := strings.Split(v, ":")
		if len(s) != 2 {
			log.Fatal("Malformed serben: ", s)
		}
		port, err := strconv.Atoi(s[1])
		if err != nil {
			log.Fatal("Invalid port: ", s[1])
		}
		log.Println("Found serben ", k, " with ip ", s[0], " on port ", port)
		servers[k] = mcpinger.New(s[0], uint16(port))
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		keys := make([]string, 0, len(servers))
		for k := range servers {
			keys = append(keys, k)
		}
		json.NewEncoder(w).Encode(keys)
	})

	for k, _ := range servers {
		http.HandleFunc("/"+k, view)
	}

	log.Fatal(http.ListenAndServe(":8001", nil))
}

func view(w http.ResponseWriter, r *http.Request) {
	if pinger, ok := servers[r.URL.EscapedPath()[1:]]; ok {
		info, _ := pinger.Ping()
		s := &Serben{info.Players.Online, info.Players.Max}
		json.NewEncoder(w).Encode(s)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}
