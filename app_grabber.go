package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json" // so we can ignore our non root CA on EH appliance
	"fmt"
	"io/ioutil"
	"log"
	"net/http" // for making HTTP requests
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	ehops  = make(map[string]string)
	APIKey = "none"
	Path   = "none"
)

func terminate(message string) {
	log.Fatal(message)
}
func terminatef(message string, v ...interface{}) {
	log.Fatalf(message, v...)
}
func getKeys() {
	keyfile, err := ioutil.ReadFile("keys")
	if err != nil {
		terminatef("Could not find keys file", err.Error())
	} else if err := json.NewDecoder(bytes.NewReader(keyfile)).Decode(&ehops); err != nil {
		terminatef("Keys file is in wrong format", err.Error())
	} else {
		for key, value := range ehops {
			APIKey = "ExtraHop apikey=" + value
			Path = "https://" + key + "/api/v1/"
			ehops[key] = value
		}
	}
}

func CreateEhopRequest(method string, call string, payload string) *http.Response {
	//Create a 'transport' object... this is necessary if we want to ignore
	//the EH insecure CA.  Similar to '--insecure' option for curl
	if APIKey == "none" {
		log.Fatal("No key file set")
		os.Exit(0)
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	//Crate a new client object... and pass him the parameters of the transport object
	//we created above
	client := http.Client{Transport: tr}
	postBody := []byte(payload)
	req, err := http.NewRequest(method, Path+call, bytes.NewBuffer(postBody))
	if err != nil {
		terminatef("Failed to create HTTP request: %q", err.Error())
	}

	//Add some header stuff to make it EH friendly
	req.Header.Add("Authorization", APIKey)
	req.Header.Add("Content-Type", " application/json")
	resp, err := client.Do(req)
	if err != nil {
		terminatef("Failed to perform HTTP request: %q", err.Error())
	}
	return resp
}
func ConvertResponseToJSONArray(resp *http.Response) []map[string]interface{} {
	// Depending on the request, you may not need an array
	//var results = make(map[string]interface{})
	var mapp = make([]map[string]interface{}, 0)
	if err := json.NewDecoder(resp.Body).Decode(&mapp); err != nil {
		terminatef("Could not parse results: %q", err.Error())
	}
	defer resp.Body.Close()
	return mapp
}
func ConvertResponseToJSON(resp *http.Response) map[string]interface{} {
	// Depending on the request, you may not need an array
	//var results = make(map[string]interface{})
	var mapp = make(map[string]interface{}, 0)
	if err := json.NewDecoder(resp.Body).Decode(&mapp); err != nil {
		terminatef("Could not parse results: %q", err.Error())
	}
	defer resp.Body.Close()
	return mapp
}

func tagger(tag string, servers map[float64]string) {
	// create tag
	body := `{ "name": "` + tag + `" }`
	response := CreateEhopRequest("POST", "tags", body)
	index := strings.LastIndex(response.Header.Get("location"), "/")
	tagID := response.Header.Get("location")[index+1:]
	body = `{ "assign": [` + tagID + `], "unassign": [] }`
	for device, _ := range servers {
		CreateEhopRequest("POST", "devices/"+strconv.FormatFloat(device, 'f', 0, 64)+"/tags", body)
	}
	body = `{ "description": "", "dynamic": true, "field": "tag", "include_custom_devices": false, "name": "` + tag + `", "value": "` + tag + `" }`
	response = CreateEhopRequest("POST", "devicegroups", body)
}
func ServerFinder(metricCategory string, search string) map[float64]string {
	var mapp = make(map[float64]string)
	var hits = make(map[float64]string)
	body := `{ "description": "temp device group", "dynamic": true, "field": "type", "include_custom_devices": false, "name": "temp_HTTP", "value": "/^extrahop.device.http_server$/" }`
	response := CreateEhopRequest("POST", "devicegroups", body)
	time.Sleep(1000 * time.Millisecond)
	index := strings.LastIndex(response.Header.Get("location"), "/")
	groupID := response.Header.Get("location")[index+1:]
	devices := ConvertResponseToJSONArray(CreateEhopRequest("GET", "devicegroups/"+groupID+"/devices?limit=5000", ""))
	for _, device := range devices {
		if device["ipaddr4"] != nil {
			mapp[device["id"].(float64)] = device["ipaddr4"].(string)
		}
	}
	CreateEhopRequest("DELETE", "devicegroups/"+groupID, "")

	for key := range mapp {
		fmt.Printf("Puling Stats for Server %s\n", mapp[key])
		body = ` { "cycle": "1hr", "from": -86400000, "metric_category": "` + metricCategory + `", "metric_specs": [{ "name": "req" }], "object_ids": [` + strconv.FormatFloat(key, 'f', 0, 64) + `], "object_type": "device", "until": 0 }`
		stats := ConvertResponseToJSON(CreateEhopRequest("POST", "metrics", body))
		for _, value := range stats["stats"].([]interface{}) {
			for key2, value2 := range value.(map[string]interface{}) {
				if key2 == "values" {
					for _, value3 := range value2.([]interface{}) {
						for _, value4 := range value3.([]interface{}) {
							for key5, value5 := range value4.(map[string]interface{}) {
								if key5 == "key" {
									for key6, value6 := range value5.(map[string]interface{}) {
										if key6 == "str" {
											if strings.Contains(value6.(string), search) {
												hits[key] = mapp[key]
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return hits
}

func main() {
	getKeys()
	servers := ServerFinder("uri_http_server_detail", "1400/SystemProperties/Control")
	tagger("Sonos", servers)

}
