package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"

	"github.com/gorilla/mux"
	"github.com/satori/go.uuid"
	"github.com/warthog618/goatsms/internal/db"
	"github.com/warthog618/goatsms/internal/sender"
)

// SMSResponse is the response structure to /sms requests.
type SMSResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// SMSDataResponse defines the response structure to /smsdata/ requests.
type SMSDataResponse struct {
	Status   int            `json:"status"`
	Message  string         `json:"message"`
	Summary  []int          `json:"summary"`
	DayCount map[string]int `json:"daycount"`
	Messages []db.SMS       `json:"messages"`
}

/* dashboard handlers */

// dashboard
func indexHandler() func(w http.ResponseWriter, r *http.Request) {
	t := template.Must(template.ParseFiles("./templates/index.html"))
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("--- indexHandler")
		// Use during development to avoid having to restart server
		// after every change in HTML
		//t, _ = template.ParseFiles("./templates/index.html")
		t.Execute(w, nil)
	}
}

// staticHandler handles all static files based on specified path
// for now its /assets
func staticHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	static := vars["path"]
	http.ServeFile(w, r, filepath.Join("./assets", static))
}

/* end dashboard handlers */

/* API handlers */

// sendSMSHandler push sms, allowed methods: POST
func sendSMSHandler(s *sender.Sender) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("--- sendSMSHandler")
		w.Header().Set("Content-type", "application/json")

		//TODO: validation
		r.ParseForm()
		mobile := r.FormValue("mobile")
		message := r.FormValue("message")
		uuid := uuid.NewV1()
		s.AddMessage(db.SMS{UUID: uuid.String(), Mobile: mobile, Body: message})

		smsresp := SMSResponse{Status: 200, Message: "ok"}
		toWrite, err := json.Marshal(smsresp)
		if err != nil {
			log.Println(err)
			//lets just depend on the server to raise 500
		}
		w.Write(toWrite)
	}
}

// getLogsHandler dumps JSON data, used by log view. Methods allowed: GET
func getLogsHandler(d *db.DB) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("--- getLogsHandler")
		messages, _ := d.GetMessages("")
		summary, _ := d.GetStatusSummary()
		dayCount, _ := d.GetLast7DaysMessageCount()
		logs := SMSDataResponse{
			Status:   200,
			Message:  "ok",
			Summary:  summary,
			DayCount: dayCount,
			Messages: messages,
		}
		toWrite, err := json.Marshal(logs)
		if err != nil {
			log.Println(err)
			//lets just depend on the server to raise 500
		}
		w.Header().Set("Content-type", "application/json")
		w.Write(toWrite)
	}
}

/* end API handlers */

// InitServer runs a http server.
func InitServer(d *db.DB, s *sender.Sender, host string, port string) error {
	log.Println("--- InitServer ", host, port)

	r := mux.NewRouter()
	r.StrictSlash(true)

	r.HandleFunc("/", indexHandler())

	// handle static files
	r.HandleFunc(`/assets/{path:[a-zA-Z0-9=\-\/\.\_]+}`, staticHandler)

	// all API handlers
	api := r.PathPrefix("/api").Subrouter()

	api.Methods("GET").Path("/logs/").HandlerFunc(getLogsHandler(d))
	api.Methods("POST").Path("/sms/").HandlerFunc(sendSMSHandler(s))

	http.Handle("/", r)

	bind := fmt.Sprintf("%s:%s", host, port)
	log.Println("listening on: ", bind)
	return http.ListenAndServe(bind, nil)
}
