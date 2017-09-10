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

// Cache templates
var templates = template.Must(template.ParseFiles("./templates/index.html"))

// !!! These package globals are evil and should be refactored away.
var dispatcher *sender.Sender
var smsdb *db.DB

/* dashboard handlers */

// dashboard
func indexHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("--- indexHandler")
	// templates.ExecuteTemplate(w, "index.html", nil)
	// Use during development to avoid having to restart server
	// after every change in HTML
	t, _ := template.ParseFiles("./templates/index.html")
	t.Execute(w, nil)
}

// handle all static files based on specified path
// for now its /assets
func handleStatic(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	static := vars["path"]
	http.ServeFile(w, r, filepath.Join("./assets", static))
}

/* end dashboard handlers */

/* API handlers */

// sendSMSHandler push sms, allowed methods: POST
func sendSMSHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("--- sendSMSHandler")
	w.Header().Set("Content-type", "application/json")

	//TODO: validation
	r.ParseForm()
	mobile := r.FormValue("mobile")
	message := r.FormValue("message")
	uuid := uuid.NewV1()
	dispatcher.AddMessage(db.SMS{UUID: uuid.String(), Mobile: mobile, Body: message})

	smsresp := SMSResponse{Status: 200, Message: "ok"}
	var toWrite []byte
	toWrite, err := json.Marshal(smsresp)
	if err != nil {
		log.Println(err)
		//lets just depend on the server to raise 500
	}
	w.Write(toWrite)
}

// getLogsHandler dumps JSON data, used by log view. Methods allowed: GET
func getLogsHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("--- getLogsHandler")
	// !!! bundle into a single call into gosms
	// Or split the API into several levels... (or both)
	messages, _ := smsdb.GetMessages("")
	summary, _ := smsdb.GetStatusSummary()
	dayCount, _ := smsdb.GetLast7DaysMessageCount()
	logs := SMSDataResponse{
		Status:   200,
		Message:  "ok",
		Summary:  summary,
		DayCount: dayCount,
		Messages: messages,
	}
	var toWrite []byte
	toWrite, err := json.Marshal(logs)
	if err != nil {
		log.Println(err)
		//lets just depend on the server to raise 500
	}
	w.Header().Set("Content-type", "application/json")
	w.Write(toWrite)
}

/* end API handlers */

// InitServer runs a http server.
func InitServer(store *db.DB, sender *sender.Sender, host string, port string) error {
	log.Println("--- InitServer ", host, port)

	smsdb = store
	dispatcher = sender

	r := mux.NewRouter()
	r.StrictSlash(true)

	r.HandleFunc("/", indexHandler)

	// handle static files
	r.HandleFunc(`/assets/{path:[a-zA-Z0-9=\-\/\.\_]+}`, handleStatic)

	// all API handlers
	api := r.PathPrefix("/api").Subrouter()
	api.Methods("GET").Path("/logs/").HandlerFunc(getLogsHandler)
	api.Methods("POST").Path("/sms/").HandlerFunc(sendSMSHandler)

	http.Handle("/", r)

	bind := fmt.Sprintf("%s:%s", host, port)
	log.Println("listening on: ", bind)
	return http.ListenAndServe(bind, nil)

}
