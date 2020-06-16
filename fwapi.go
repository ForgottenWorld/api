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

	http.HandleFunc("/serben/", view)

	log.Fatal(http.ListenAndServe(":8001", nil))
}

func view(w http.ResponseWriter, r *http.Request) {
	k := r.URL.EscapedPath()[len("/serben/"):]
	if pinger, ok := servers[k]; ok {
		info, _ := pinger.Ping()
		if info == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		s := &Serben{info.Players.Online, info.Players.Max}
		json.NewEncoder(w).Encode(s)
		return
	}
	http.NotFound(w, r)
}
