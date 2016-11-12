/**
 * Copyright (c) Mainflux
 *
 * Mainflux server is licensed under an Apache license, version 2.0.
 * All rights not explicitly granted in the Apache license, version 2.0 are reserved.
 * See the included LICENSE file for more details.
 */

package api

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/mainflux/mainflux-core/db"
	"github.com/mainflux/mainflux-core/models"

	"github.com/satori/go.uuid"
	"gopkg.in/mgo.v2/bson"

	"github.com/krylovsk/gosenml"

	"io"
	"io/ioutil"
	"net/http"

	"github.com/go-zoo/bone"
)

type (
	// ChannelWriteStatus is a type of Go chan
	// that is used to communicate request status
	ChannelWriteStatus struct {
		Nb  int
		Str string
	}
)

/** == Functions == */

// createChannel function
func createChannel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	if len(data) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		str := `{"response": "no data (Device ID) provided"}`
		io.WriteString(w, str)
		return
	}

	var body map[string]interface{}
	if err := json.Unmarshal(data, &body); err != nil {
		panic(err)
	}

	/**
	if validateJsonSchema("channel", body) != true {
		println("Invalid schema")
		w.WriteHeader(http.StatusBadRequest)
		str := `{"response": "invalid json schema in request"}`
		io.WriteString(w, str)
		return
	}
	**/

	// Init new Mongo session
	// and get the "channels" collection
	// from this new session
	Db := db.MgoDb{}
	Db.Init()
	defer Db.Close()

	c := models.Channel{}
	if err := json.Unmarshal(data, &c); err != nil {
		panic(err)
	}

	// Creating UUID Version 4
	uuid := uuid.NewV4()
	fmt.Println(uuid.String())

	c.ID = uuid.String()

	// Insert reference to DeviceID
	if len(c.Device) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		str := `{"response": "no device ID provided in request"}`
		io.WriteString(w, str)
		return
	}

	// TODO Check if Device ID is valid (in database)

	// Timestamp
	t := time.Now().UTC().Format(time.RFC3339)
	c.Created, c.Updated = t, t

	// Insert Channel
	if err := Db.C("channels").Insert(c); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		str := `{"response": "cannot create channel"}`
		io.WriteString(w, str)
		return
	}

	w.WriteHeader(http.StatusOK)
	str := `{"response": "created", "id": "` + c.ID + `"}`
	io.WriteString(w, str)
}

// getChannels function
func getChannels(w http.ResponseWriter, r *http.Request) {
	Db := db.MgoDb{}
	Db.Init()
	defer Db.Close()

	// Get fileter values from parameters:
	// - climit = count limit, limits number of returned `channel` elements
	// - vlimit = value limit, limits number of values within the channel
	var climit, vlimit int
	var err error
	s := r.URL.Query().Get("climit")
	if len(s) == 0 {
		// Set default limit to -5
		climit = -100
	} else {
		climit, err = strconv.Atoi(s)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			str := `{"response": "wrong count limit"}`
			io.WriteString(w, str)
			return
		}
	}

	s = r.URL.Query().Get("vlimit")
	if len(s) == 0 {
		// Set default limit to -5
		vlimit = -100
	} else {
		vlimit, err = strconv.Atoi(s)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			str := `{"response": "wrong value limit"}`
			io.WriteString(w, str)
			return
		}
	}

	// Query DB
	results := []models.Channel{}
	if err := Db.C("channels").Find(nil).
		Select(bson.M{"values": bson.M{"$slice": vlimit}}).
		Sort("-_id").Limit(climit).All(&results); err != nil {
		log.Print(err)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	res, err := json.Marshal(results)
	if err != nil {
		log.Print(err)
	}
	io.WriteString(w, string(res))
}

// getChannel function
func getChannel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	Db := db.MgoDb{}
	Db.Init()
	defer Db.Close()

	id := bone.GetValue(r, "channel_id")

	var vlimit int
	var err error
	s := r.URL.Query().Get("vlimit")
	if len(s) == 0 {
		// Set default limit to -5
		vlimit = -5
	} else {
		vlimit, err = strconv.Atoi(s)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			str := `{"response": "wrong limit"}`
			io.WriteString(w, str)
			return
		}
	}

	result := models.Channel{}
	if err := Db.C("channels").Find(bson.M{"id": id}).
		Select(bson.M{"values": bson.M{"$slice": vlimit}}).
		One(&result); err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusNotFound)
		str := `{"response": "not found", "id": "` + id + `"}`
		io.WriteString(w, str)
		return
	}

	w.WriteHeader(http.StatusOK)
	res, err := json.Marshal(result)
	if err != nil {
		log.Print(err)
	}
	io.WriteString(w, string(res))
}

// writeChannel function
// Generic function that updates the channel value.
// Can be called via various protocols.
func writeChannel(id string, bodyBytes []byte) {
	var body map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		fmt.Println("Error unmarshaling body")
	}

	Db := db.MgoDb{}
	Db.Init()
	defer Db.Close()

	s := ChannelWriteStatus{}

	// Check if someone is trying to change "id" key
	// and protect us from this
	if _, ok := body["id"]; ok {
		s.Nb = http.StatusBadRequest
		s.Str = "Invalid request: 'id' is read-only"
		fmt.Println(s.Nb, s.Str)
		return
	}
	if _, ok := body["device"]; ok {
		println("Error: can not change device")
		s.Nb = http.StatusBadRequest
		s.Str = "Invalid request: 'device' is read-only"
		fmt.Println(s.Nb, s.Str)
		return
	}
	if _, ok := body["created"]; ok {
		println("Error: can not change device")
		s.Nb = http.StatusBadRequest
		s.Str = "Invalid request: 'created' is read-only"
		fmt.Println(s.Nb, s.Str)
		return
	}

	// Find the channel
	c := models.Channel{}
	if err := Db.C("channels").Find(bson.M{"id": id}).One(&c); err != nil {
		s.Nb = http.StatusNotFound
		s.Str = "Channel not found"
		fmt.Println(s.Nb, s.Str)
		return
	}

	senmlDecoder := gosenml.NewJSONDecoder()
	var m gosenml.Message
	var err error
	if m, err = senmlDecoder.DecodeMessage(bodyBytes); err != nil {
		s.Nb = http.StatusBadRequest
		s.Str = "Invalid request: SenML can not be decoded"
		fmt.Println(s.Nb, s.Str)
		return
	}

	m.BaseName = c.Name + m.BaseName
	m.BaseUnits = c.Unit + m.BaseUnits

	for _, e := range m.Entries {
		// Name = channelName + baseName + entryName
		e.Name = m.BaseName + e.Name

		// BaseTime
		e.Time = m.BaseTime + e.Time
		if e.Time <= 0 {
			e.Time += time.Now().Unix()
		}

		// BaseUnits
		if e.Units == "" {
			e.Units = m.BaseUnits
		}

		/** Insert entry in DB */
		colQuerier := bson.M{"id": id}
		change := bson.M{"$push": bson.M{"values": e}}
		err := Db.C("channels").Update(colQuerier, change)
		if err != nil {
			log.Print(err)
			s.Nb = http.StatusNotFound
			s.Str = "Not inserted"
			fmt.Println(s.Nb, s.Str)
			return
		}
	}

	// Timestamp
	t := time.Now().UTC().Format(time.RFC3339)
	body["updated"] = t

	/** Then update channel timestamp */
	colQuerier := bson.M{"id": id}
	change := bson.M{"$set": bson.M{"updated": body["updated"]}}
	if err := Db.C("channels").Update(colQuerier, change); err != nil {
		log.Print(err)
		s.Nb = http.StatusNotFound
		s.Str = "Not updated"
		fmt.Println(s.Nb, s.Str)
		return
	}

	s.Nb = http.StatusOK
	s.Str = "Updated"

	println(s.Str)
}

// updateChannel function
func updateChannel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	Db := db.MgoDb{}
	Db.Init()
	defer Db.Close()

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	if len(data) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		str := `{"response": "no data provided"}`
		io.WriteString(w, str)
		return
	}

	var body map[string]interface{}
	if err := json.Unmarshal(data, &body); err != nil {
		panic(err)
	}

	/**
	if validateJsonSchema("channel", body) != true {
		println("Invalid schema")
		w.WriteHeader(http.StatusBadRequest)
		str := `{"response": "invalid json schema in request"}`
		io.WriteString(w, str)
		return
	}
	**/

	id := bone.GetValue(r, "channel_id")

	// Publish the channel update.
	// This will be catched by the MQTT main client (subscribed to all channel topics)
	// and then written in the DB in the MQTT handler
	token := mqttClient.Publish("mainflux/"+id, 0, false, string(data))
	token.Wait()

	// Send back response to HTTP client
	// We have accepted the request and published it over MQTT,
	// but we do not know if it will be executed or not (MQTT is not req-reply protocol)
	w.WriteHeader(http.StatusAccepted)
	str := `{"response": "channel update published"}`
	io.WriteString(w, str)
}

// deleteChannel function
func deleteChannel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	Db := db.MgoDb{}
	Db.Init()
	defer Db.Close()

	id := bone.GetValue(r, "channel_id")

	err := Db.C("channels").Remove(bson.M{"id": id})
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusNotFound)
		str := `{"response": "not deleted", "id": "` + id + `"}`
		io.WriteString(w, str)
		return
	}

	w.WriteHeader(http.StatusOK)
	str := `{"response": "deleted", "id": "` + id + `"}`
	io.WriteString(w, str)
}
