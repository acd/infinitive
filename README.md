# infinitive
Infinitive impersonates a SAM on a Carrier Infinity system management bus. 

## **DISCLAIMER**
**The software and hardware described here interacts with a proprietary system using information gleaned by reverse engineering.  Although it works fine for me, no guarantee or warranty is provided.  Use at your own risk to your HVAC system and yourself.**

## Getting started
#### Hardware setup
I've done all my development on a Raspberry Pi, although any reasonably performant Linux system with an RS-485 interface should work.  I chose the Pi 3 since the built-in WiFi saved me the hassle of running Ethernet to my furnace.  I'm not sure if older Pis have enough horsepower to run the Infinitive software.  If you give it a shot and are successful, please let me know so I can update this information.

In addition to a Linux system, you'll need to source an adapter to communicate on the RS-485 bus.  I am using a FTDI chipset USB to RS-485 adapter that I bought from Amazon.  There are a variety of adapters on Amazon and eBay, although it may take a few attempts to find one that works reliably.

Once you have a RS-485 adapter you'll need to connect it to your ABCD bus. The easiest way to do this is by attaching new wires to the A and B terminals of the ABCD bus connector inside your furnace and connecting them to your adapter. The A and B lines are used for RS-485 communication, while C and D are 24V AC power. **Do not connect your RS-485 adapter to the C and D terminals unless you want to see its magic smoke.** 

#### Software
Download the Infinitive release appropriate for your architecture.

   * amd64:
```
$ wget -O infinitive https://github.com/acd/infinitive/releases/download/v0.2/infinitive.amd64
```
   * arm:
```
$ wget -O infinitive https://github.com/acd/infinitive/releases/download/v0.2/infinitive.arm
```

Start Infinitive, providing the HTTP port to listen on for the management interface and the path to the correct serial device.

```
$ ./infinitive -httpport=8080 -serial=/dev/ttyUSB0 
```

Logs are written to stderr.  For now I've been running Infinitive under screen.  If folks are interested in a proper start/stop script and log management, submit a pull request or let me know.

If the RS-485 adapter is properly connected to your ABCD bus you should immediately see Infinitive logging messages indicating it is receiving data, such as:

```
INFO[0000] read frame: 2001 -> 4001: READ     000302    
INFO[0000] read frame: 4001 -> 2001: RESPONSE 000302041100000414000004020000 
INFO[0000] read frame: 2001 -> 4001: READ     000316    
INFO[0000] read frame: 4001 -> 2001: RESPONSE 0003160000000003ba004a2f780100037a 
```

Browse to your host system's IP, with the port you provided on the command line, and you should see a page that looks similar to the following:

<img src="https://raw.githubusercontent.com/acd/infinitive/master/screenshot.png"/>

**Note:** I am not much of a frontend web developer.  I'd love to see pull requests for enhancements to the web interface.

There is a brief delay between altering a setting and Infinitive updating the information displayed.  This is due to Infinitive polling the thermostat settings once per second.

## Building from source

If you'd like to build Infinitive from source, first confirm you have a working Go environment (I've been using release 1.7.1).  Ensure your GOPATH and GOHOME are set correctly, then:

```
$ go get github.com/acd/infinitive
$ go build github.com/acd/infinitive
```
Note: If you make changes to the code or other resources in the assets directory you will need to rebuild the bindata_assetfs.go file. You will need the go-bindata-assetfs utility.
 
1. Install go-bindata-assetfs into your go folders

Details, and installation instructions are available here.

https://github.com/elazarl/go-bindata-assetfs

2. Rebuild bindata_assetfs.go

From within the infinitive folder execute
```
$ go-bindata-assetfs assets/...
```


## JSON API

Infinitive exposes a JSON API to retrieve and manipulate thermostat parameters.

#### GET /api/zone/1/config

```json
{
   "currentTemp": 70,
   "currentHumidity": 50,
   "outdoorTemp": 50,
   "mode": "heat",
   "stage":2,
   "fanMode": "auto",
   "hold": true,
   "heatSetpoint": 68,
   "coolSetpoint": 74,
   "rawMode": 64
}
```
rawMode included for debugging purposes. It encodes stage and mode. 

#### PUT /api/zone/1/config

```json
{
   "mode": "auto",
   "fanMode": "auto",
   "hold": true,
   "heatSetpoint": 68,
   "coolSetpoint": 74
}
```

Valid write values for `mode` are `off`, `auto`, `heat`, and `cool`.
Additional read values for mode are `electric` and `heatpump` indicating "heat pump only" or "electric heat only" have been selected at the thermostat 
Values for `fanMode` are `auto`, `low`, `med`, and `high`.

#### GET /api/zone/1/airhandler

```json
{
	"blowerRPM":0,
	"airFlowCFM":0,
	"elecHeat":false
}
```

#### GET /api/zone/1/heatpump

```json
{
	"coilTemp":28.8125,
	"outsideTemp":31.375,
	"stage":2
}
```


#### GET /api/zone/1/vacation

```
{
   "active":false,
   "days":0,
   "minTemperature":56,
   "maxTemperature":84,
   "minHumidity":15,
   "maxHumidity":60,
   "fanMode":"auto"
}
```

#### PUT /api/zone/1/vacation

```
{
   "days":0,
   "minTemperature":56,
   "maxTemperature":84,
   "minHumidity":15,
   "maxHumidity":60,
   "fanMode":"auto"
}
```

All parameters are optional.  A single parameter may be updated by sending a JSON document containing only that parameter.  Vacation mode is disabled by setting `days` to `0`.  Valid values for `fanMode` are `auto`, `low`, `med`, and `high`.

## Details
#### ABCD bus
Infinity systems use a proprietary binary protocol for data exchange between system components.  These message are sent across an RS-485 serial bus which Carrier refers to as the ABCD bus.  Most systems usually includes an air-conditioning unit or heat pump, furnace, and thermostat.  The thermostat is responsible for enumerating other components of the system and managing their operation. 

The protocol has been reverse engineered as Carrier has not published a protocol specification.  The following resources provided invaluable assistance with my reverse engineering efforts:

* [Cocoontech's Carrier Infinity Thread](http://cocoontech.com/forums/topic/11372-carrier-infinity/)
* [Infinitude Wiki](https://github.com/nebulous/infinitude/wiki/Infinity-Protocol-Main)

Infinitive reads and writes information from the Infinity thermostat.  It also gathers data by passively observing traffic exchanged between the thermostat and other system components.

#### Bryant Evolution
I believe Infinitive should work with Bryant Evolution systems as they use the same ABCD bus.  Please let me know if you have success using Infinitive on a Bryant system.

#### Unimplemented features

Multi-zone Infinity HVAC systems are not supported.  I only have a single zone setup, so I can't test if multi-zone capability works properly even if I implement it.  If you have a multi-zone setup and want to be a guinea pig, get in touch and maybe we can work something out.

I don't use the thermostat's scheduling capabilities or vacation mode so Infinitive does not support them.  Reach out if this is something you'd like to see.  

#### Issues
##### rPi USB stack
The USB to RS-485 adapter I'm using periodically locks up due to what appear to be USB stack issues on the Raspberry Pi 3.  When this happens, reads on the serial file descriptor block forever and the kernel logs the following:
```
[491862.396039] ftdi_sio ttyUSB0: usb_serial_generic_read_bulk_callback - urb stopped: -32
```
Infinitive reopens the serial interface when it hasn't received any data in 5 seconds to workaround the issue.  Alternatively, forcing the Pi USB stack to USB 1.1 mode resolves the issue.  If you want to go this route, add `dwc_otg.speed=1` to `/boot/config.txt` and reboot the Pi.

##### Bogus data
Occasionally Infinitive will display incorrect data via the web interface for a second.  This is likely caused by improper parsing of data received from the ABCD bus.  I'd like to track down the root cause of this issue and resolve it, but due to its transient nature it's not a high priority and does not affect usability.

#### See Also
[Infinitude](https://github.com/nebulous/infinitude) is another solution for managing Carrier HVAC systems.  It impersonates Carrier web services and provides an alternate interface for controlling Carrier Internet-enabled touchscreen thermostats.  It also supports passive snooping of the RS-485 bus and can decode and display some of the data.

#### Contact
Andrew Danforth (<adanforth@gmail.com>)
