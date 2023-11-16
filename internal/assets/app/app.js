console.log("loaded app.js");

var app = angular.module('thermostatApp', ['ngWebSocket']);

/*
app.factory('thermostatEvents', function($websocket) {
      // Open a WebSocket connection
      var dataStream = $websocket("ws://192.168.1.37:8080/api/ws");

      var collection = [];

      dataStream.onMessage(function(message) {
        console.log("got message!");
	console.log(message);
        collection.push(JSON.parse(message.data));
      });

      var methods = {
        collection: collection,
        get: function() {
          dataStream.send(JSON.stringify({ action: 'get' }));
        }
      };

      return methods;
});
*/

app.factory('thermostatEvents', function ($websocket) {
        return {
            start: function (url, callback) {
                var s = $websocket(url, null, {reconnectIfNotNormalClose: true});
                s.onMessage(function(message) {
                  console.log("got message!");
          	  console.log(message);
                  callback(JSON.parse(message.data));
                });
            }
        }
    }
);

// Define the `PhoneListController` controller on the `phonecatApp` module
app.controller('thermostatController', function($scope, $http, $interval, $location, thermostatEvents) {
  $scope.tstat = {};
  $scope.blower = {};

  var $wsUrl = "ws://" + $location.host() + ":" + $location.port() + "/api/ws";

  thermostatEvents.start($wsUrl, function (msg) {
    if (msg.source == "tstat") {
       $scope.tstat = msg.data;
    } else if (msg.source == "blower") {
       $scope.blower = msg.data;
    }
  });

  // $scope.events = thermostatEvents;

  $scope.refreshState = function () {
     $http.get("/api/zone/1/config").then(function(response) {
      $scope.tstat = response.data;
    });
  };

  $scope.setFanSpeed = function(speed) {
    $http.put("/api/zone/1/config", { "fanMode": speed }).then(function(response) {
      console.log("set fan speed") ;
    });
  }

  $scope.setMode = function(mode) {
    $http.put("/api/zone/1/config", { "mode": mode }).then(function(response) {
      console.log("set fan speed") ;
    });
  }

  $scope.setHold = function(hold) {
    $http.put("/api/zone/1/config", { "hold": hold }).then(function(response) {
      console.log("set hold") ;
    });
  }

  $scope.incCoolSetpoint = function(val) {
    var temp = $scope.tstat.coolSetpoint + val;
    $http.put("/api/zone/1/config", { "coolSetpoint": temp }).then(function(response) {
      console.log("set fan speed") ;
    });
  }

  $scope.incHeatSetpoint = function(val) {
    var temp = $scope.tstat.heatSetpoint + val;
    $http.put("/api/zone/1/config", { "heatSetpoint": temp }).then(function(response) {
      console.log("set fan speed") ;
    });
  }

/*
  $interval(function () {
     $scope.refreshState();
  }, 1000);
*/
});
