package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var hostValidMatch = regexp.MustCompile("^(([a-zA-Z]|[a-zA-Z][a-zA-Z0-9\\-]*[a-zA-Z0-9])\\.)*([A-Za-z]|[A-Za-z][A-Za-z0-9\\-]*[A-Za-z0-9])$")
var hostWwwPrefix = regexp.MustCompile("^www\\.")
var recordMatch = regexp.MustCompile("^\\s*((\\d+)\\s+(\\S+)\\s+)?(\\S+)(.*)$")

type listByOrder []*Record

func (l listByOrder) Len() int           { return len(l) }
func (l listByOrder) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l listByOrder) Less(i, j int) bool { return l[i].Order < l[j].Order }

type Record struct {
	Order  int
	Match  string
	Target string
}

func main() {
	http.HandleFunc("/", requestHandler)

	failure := make(chan error, 1)

	go func(failure chan error) {
		failure <- http.ListenAndServe(":80", nil)
	}(failure)

	go func(failure chan error) {
		//	failure <- http.ListenAndServeTLS(":443", "ssl.crt", "ssl.key", nil)
	}(failure)

	fmt.Println(<-failure)
	os.Exit(1)
}

func requestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Server", "MegagramRedirector/1.0")
	if r.URL.RequestURI() == "/favicon.ico" {
		return
	}
	if !hostValidMatch.MatchString(r.Host) {
		fmt.Fprintln(w, "<h1>Invalid Hostname</h1><p>This domain is not in a valid format for this service.</p>")
		return
	}
	r.URL.Host = r.Host
	r.URL.Scheme = "http"
	if r.TLS != nil {
		r.URL.Scheme = "https"
	}
	txts, err := net.LookupTXT(fmt.Sprintf("_tgt._redir.%s", r.Host))
	if err != nil {
		fmt.Fprintf(w, "<h1>ERROR OCCURRED</h1><pre>%s</pre>\n", err)
		return
	}
	if len(txts) == 0 && !hostWwwPrefix.MatchString(r.URL.Host) {
		r.URL.Host = fmt.Sprintf("www.%s", r.URL.Host)
		http.Redirect(w, r, r.URL.String(), http.StatusFound)
		return
	}
	if len(txts) > 0 {
		var records = make(listByOrder, len(txts))
		i := 0
		for _, txt := range txts {
			parts := recordMatch.FindAllStringSubmatch(txt, 1)
			if len(parts) > 0 {
				record := new(Record)
				if parts[0][2] == "" {
					parts[0][2] = "99999"
				}
				//	if parts[0][3] == "" {
				//		parts[0][3] = ""
				//	}
				record.Order, _ = strconv.Atoi(parts[0][2])
				record.Match = parts[0][3]
				record.Target = parts[0][4]
				record.Target = strings.Replace(record.Target, "${SCHEME}", r.URL.Scheme, -1)
				record.Target = strings.Replace(record.Target, "${HOST}", r.URL.Host, -1)
				record.Target = strings.Replace(record.Target, "${PATH}", r.URL.Path, -1)
				record.Target = strings.Replace(record.Target, "${QUERY}", r.URL.RawQuery, -1)
				records[i] = record
				i++
			}
		}
		if len(records) > 0 {
			sort.Sort(records)
			for _, record := range records {
				//	fmt.Printf("[%d] [%d] [%s] [%s]\n", i, record.Order, record.Match, record.Target)
				match, err := regexp.Compile(record.Match)
				if err != nil {
					fmt.Fprintln(w, "<h1>Match Rule Malformed</h1><p>This domain has a malformed match rule.</p>")
					return
				}
				matches := match.FindAllStringSubmatch(r.URL.String(), 1)
				if len(matches) > 0 {
					parts := matches[0][1:]
					partsInterface := make([]interface{}, len(parts))
					for i, v := range parts {
						partsInterface[i] = v
					}
					http.Redirect(w, r, fmt.Sprintf(record.Target, partsInterface...), http.StatusFound)
					return
				}
			}
			if !hostWwwPrefix.MatchString(r.URL.Host) {
				r.URL.Host = fmt.Sprintf("www.%s", r.URL.Host)
				http.Redirect(w, r, r.URL.String(), http.StatusFound)
				return
			}
			fmt.Fprintln(w, "<h1>No Match</h1><p>This domain is configured, but none of the rules matched.</p>")
			return
		}
		fmt.Fprintln(w, "<h1>No Valid Rules</h1><p>This domain is configured, but none of the rules were formatted correctly.</p>")
		return
	}
	fmt.Fprintln(w, "<h1>Unconfigured</h1><p>This URL is unconfigured <i>or</i> is not valid in the system.</p>")
}
