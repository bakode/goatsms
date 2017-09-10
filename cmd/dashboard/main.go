package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/warthog618/goatsms"
	"github.com/warthog618/goatsms/internal/db"
	"github.com/warthog618/goatsms/internal/modem"
	"github.com/warthog618/goatsms/internal/sender"
)

func main() {

	log.Println("main: ", "Initializing gosms")
	//load the config, abort if required config is not preset
	appConfig, err := gosms.GetConfig("conf.ini")
	if err != nil {
		log.Println("main: ", "Invalid config: ", err.Error(), " Aborting")
		os.Exit(1)
	}

	store, err := db.New("sqlite3", "db.sqlite")
	if err != nil {
		log.Println("main: ", "Error initializing database: ", err, " Aborting")
		os.Exit(1)
	}
	defer store.Close()

	serverhost, _ := appConfig.Get("SETTINGS", "SERVERHOST")
	serverport, _ := appConfig.Get("SETTINGS", "SERVERPORT")

	serverusername, _ := appConfig.Get("SETTINGS", "USERNAME")
	serverpassword, _ := appConfig.Get("SETTINGS", "PASSWORD")

	_numDevices, _ := appConfig.Get("SETTINGS", "DEVICES")
	numDevices, _ := strconv.Atoi(_numDevices)
	log.Println("main: number of modems: ", numDevices)

	modems := make([]*modem.GSMModem, numDevices)
	for i := 0; i < numDevices; i++ {
		dev := fmt.Sprintf("DEVICE%v", i)
		_port, _ := appConfig.Get(dev, "COMPORT")
		_baud := 115200 //appConfig.Get(dev, "BAUDRATE")
		_devid, _ := appConfig.Get(dev, "DEVID")
		modems[i] = modem.New(_port, _baud, _devid)
	}

	_bufferSize, _ := appConfig.Get("SETTINGS", "BUFFERSIZE")
	bufferSize, _ := strconv.Atoi(_bufferSize)

	_bufferLow, _ := appConfig.Get("SETTINGS", "BUFFERLOW")
	bufferLow, _ := strconv.Atoi(_bufferLow)

	//_loaderTimeout, _ := appConfig.Get("SETTINGS", "MSGTIMEOUT")
	//loaderTimeout, _ := strconv.Atoi(_loaderTimeout)

	//_loaderCountout, _ := appConfig.Get("SETTINGS", "MSGCOUNTOUT")
	//loaderCountout, _ := strconv.Atoi(_loaderCountout)

	_loaderTimeoutLong, _ := appConfig.Get("SETTINGS", "MSGTIMEOUTLONG")
	loaderTimeoutLong, _ := time.ParseDuration(_loaderTimeoutLong + "m")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Println("main: Initializing sender")
	s := sender.New(bufferSize, bufferLow)
	go s.Run(ctx, store, loaderTimeoutLong)

	log.Println("main: Initializing modems")
	for _, m := range modems {
		m.Connect(ctx, s)
	}

	log.Println("main: Initializing server")
	err = InitServer(store, s, serverhost, serverport, serverusername, serverpassword)
	if err != nil {
		log.Println("main: ", "Error starting server: ", err.Error(), " Aborting")
		os.Exit(1)
	}
}
