/// <reference path="typings/jquery.d.ts" />
/// <reference path="typings/swfobject.d.ts" />
/// <reference path="typings/youtube.d.ts" />

function initYouTube(videoId: string): void {
    var params = { allowScriptAccess: 'always', allowFullScreen: 'true' };
    var attrs = { id: 'ytapiplayer' };
    swfobject.embedSWF("https://www.youtube.com/v/" + videoId +
        "?hl=en_US&rel=0&hd=1&border=0&version=3&fs=1&autoplay=1&autohide=0&enablejsapi=1&playerapiid=ytapiplayer",
        'ytapiplayer', '640', '360', '8', null, null, params, attrs);
}

function onYouTubePlayerReady(id) {
    console.log("YouTube player is ready.");
}

interface BrowserMessage {
	Type: string
	Message: string
}

function buildMessage(type: string, contents: any): BrowserMessage {
	return {
        Type: type,
        Message: JSON.stringify(contents)
	};
}

function handleMessage(message: BrowserMessage): void {
	console.log(message)
}

$(document).ready(() => {
    var socket = new WebSocket('wss://' + window.location.host + '/browserSocket');
    socket.onmessage = (event) => {
    	var message = <BrowserMessage>JSON.parse(event.data);
    	if (message.Type == "BrowserConnectAck") {
            var ack = JSON.parse(message.Message);
            if (ack.Success) {
                console.log("Successfully connected.")
                window.setInterval(() => {
                    console.log("Sending heartbeat.")
                    socket.send(JSON.stringify(buildMessage("Heartbeat", {Random: Math.round(Math.random()*100000)})))
                }, 5000)
            	socket.onmessage = (event) => handleMessage(<BrowserMessage>JSON.parse(event.data));
    			socket.onerror = (error) => alert("WebSocket error occurred. Connection lost.");
    		} else {
    			alert("Failed to connect to client: " + ack.Reason);
    		}
        } else {
            console.error("Got something other than a BrowserConnectAck from client.");
        }
    }
    //initYouTube("dQw4w9WgXcQ") // Never gonna give you up...
});
