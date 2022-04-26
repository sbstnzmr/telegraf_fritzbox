package main

// fritzbox.go

// Copyright 2022 Sebastian Zimmer, original code in main.go taken from
// https://github.com/cite/telegraf_fritzbox/, original copyright
// notice:
//
// Copyright 2019 Stefan Förster, original code in main.go taken from
// https://github.com/ndecker/fritzbox_exporter, original copyright
// notice:
//
// Copyright 2016 Nils Decker
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"sbstnzmr.de/fritz-status/upnp"
)

var Actions = []*ServiceActions{
	{
		Service: "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1",
		Action:  "GetTotalPacketsReceived",
		Result:  "TotalPacketsReceived",
		Name:    "packets_received",
	},
	{
		Service: "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1",
		Action:  "GetTotalPacketsSent",
		Result:  "TotalPacketsSent",
		Name:    "packets_sent",
	},
	{
		Service: "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1",
		Action:  "GetAddonInfos",
		Result:  "TotalBytesReceived",
		Name:    "bytes_received",
	},
	{
		Service: "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1",
		Action:  "GetAddonInfos",
		Result:  "TotalBytesSent",
		Name:    "bytes_sent",
	},

	{
		Service: "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1",
		Action:  "GetAddonInfos",
		Result:  "ByteSendRate",
		Name:    "bytes_send_rate",
	},
	{
		Service: "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1",
		Action:  "GetAddonInfos",
		Result:  "ByteReceiveRate",
		Name:    "bytes_receive_rate",
	},
	{
		Service: "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1",
		Action:  "GetAddonInfos",
		Result:  "PacketSendRate",
		Name:    "packet_send_rate",
	},
	{
		Service: "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1",
		Action:  "GetAddonInfos",
		Result:  "PacketReceiveRate",
		Name:    "packet_receive_rate",
	},
	{
		Service: "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1",
		Action:  "GetAddonInfos",
		Result:  "NewX_AVM_DE_TotalBytesSent64",
		Name:    "total_bytes_sent_64",
	},
	{
		Service: "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1",
		Action:  "GetAddonInfos",
		Result:  "NewX_AVM_DE_TotalBytesReceived64",
		Name:    "total_bytes_received_64",
	},

	{
		Service: "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1",
		Action:  "GetCommonLinkProperties",
		Result:  "PhysicalLinkStatus",
		Name:    "link_status",
	},
	{
		Service: "urn:schemas-upnp-org:service:WANIPConnection:1",
		Action:  "GetStatusInfo",
		Result:  "ConnectionStatus",
		Name:    "connection_status",
	},
	{
		Service: "urn:schemas-upnp-org:service:WANIPConnection:1",
		Action:  "GetStatusInfo",
		Result:  "LastConnectionError",
		Name:    "last_connection_error",
	},
	{
		Service: "urn:schemas-upnp-org:service:WANIPConnection:1",
		Action:  "GetStatusInfo",
		Result:  "Uptime",
		Name:    "uptime",
	},
}

type Fritzbox struct {
	Host string
	Port uint16
}

type ServiceActions struct {
	Service string
	Action  string
	Result  string
	Name    string
}

type Result struct {
	Name  string
	Value string
}

type ServiceResults struct {
	Name    string
	Results []Result
}

func (r *Result) influxString() string {
	// Format:
	// some_int=23i
	// some_float=32.3
	// some_bool=false
	// some_string="some string"
	var res string

	_, err := strconv.Atoi(r.Value)
	if err != nil {
		_, err := strconv.ParseFloat(r.Value, 64)
		if err != nil {
			res = fmt.Sprintf("\"%s\"", r.Value)
		} else {
			res = r.Value
		}
	} else {
		res = fmt.Sprintf("%si", r.Value)
	}

	return fmt.Sprintf("%s=%s", r.Name, res)
}

func (sr *ServiceResults) influxString(bucket string, host string) string {
	// Format:
	// fritzbox,host="192.168.178.1",source=wan some_int=23i,some_float=32.3,some_bool=false,some_string="some string"
	prefix := fmt.Sprintf("%s,host=\"%s\",source=%s ", bucket, host, sr.Name)
	influxResults := make([]string, 0)
	for _, r := range sr.Results {
		if r.Value == "<nil>" {
			continue
		}
		influxResults = append(influxResults, r.influxString())
	}
	metrics := strings.Join(influxResults, ",")

	return prefix + metrics
}

func main() {

	host := os.Getenv("FRITZBOX_HOST")
	port, _ := strconv.ParseUint(os.Getenv("FRITZBOX_PORT"), 0, 16)
	if host == "" {
		host = "192.168.178.1"
	}
	if port == 0 {
		port = 49000
	}

	root, err := upnp.LoadServices(host, uint16(port))
	if err != nil {
		log.Fatalf("fritzbox: unable to load services: %v/n", err)
	}

	//remember what we already called
	var last_service string
	var last_method string
	var result upnp.Result

	// https://github.com/influxdata/telegraf/blob/master/plugins/processors/execd/README.md
	// https://github.com/influxdata/telegraf/tree/master/plugins/inputs/execd

	reader := bufio.NewReader(os.Stdin)
	for {
		// Block and wait for telegraf to signal for the next iteration
		reader.ReadString('\n')

		// Currently we only have wan stats, no need to split
		var sr = ServiceResults{
			Name:    "wan",
			Results: make([]Result, 0),
		}
		for _, m := range Actions {
			if m.Service != last_service || m.Action != last_method {
				service, ok := root.Services[m.Service]
				if !ok {
					log.Printf("W! Cannot find defined service %s/n", m.Service)
					continue
				}
				action, ok := service.Actions[m.Action]
				if !ok {
					log.Printf("W! Cannot find defined action %s on service %s/n", m.Action, m.Service)
					continue
				}

				result, err = action.Call()
				if err != nil {
					log.Printf("E! Unable to call action %s on service %s: %v/n", m.Action, m.Service, err)
					continue
				}

				// save service and action
				last_service = m.Service
				last_method = m.Action
			}

			r := Result{
				Name:  m.Name,
				Value: fmt.Sprint(result[m.Result]),
			}
			sr.Results = append(sr.Results, r)
		}

		fmt.Println(sr.influxString("fritzbox", host))
	}
}
