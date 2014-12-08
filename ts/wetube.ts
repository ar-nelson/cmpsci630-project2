/// <reference path="typings/jquery.d.ts" />
/// <reference path="typings/swfobject.d.ts" />
/// <reference path="typings/youtube.d.ts" />

var socket: WebSocket
var myId: number
var myName: string = "(Client Name)"
var myRank: Rank = Rank.Unknown
var videoId: string = "M7lc1UVf-VE"
var heartbeat: number
var timeout: number
var ytplayer: YT.Player = null

enum Rank {
	Unknown = 0,
	Viewer,
	Editor,
	Director
}

interface BrowserMessage {
	Type: string
	Message: string
}

interface Peer {
	Id: number
	Name: string
	Address: string
	Rank: Rank
	PublicKey: string
}

function initYouTube(videoId: string): void {
    var params = { allowScriptAccess: 'always', allowFullScreen: 'true' };
    var attrs = { id: 'ytapiplayer' };
    swfobject.embedSWF("https://www.youtube.com/v/" + videoId +
        "?hl=en_US&rel=0&hd=1&border=0&version=3&fs=1&autoplay=0&autohide=0&enablejsapi=1&playerapiid=ytapiplayer",
        'ytapiplayer', '640', '360', '8', null, null, params, attrs);
}

function onYouTubePlayerReady(id): void {
    console.log("YouTube player is ready.");
    ytplayer = <YT.Player><any>document.getElementById("ytapiplayer");
    ytplayer.addEventListener('onStateChange', 'onYouTubePlayerStateChange');
}

function onYouTubePlayerStateChange(newState): void {
	console.log("Player state: " + newState);
}

function buildMessage(type: string, contents: any): BrowserMessage {
	return {
        Type: type,
        Message: JSON.stringify(contents)
	};
}

function setMyRank(newRank: Rank): void {
	myRank = newRank;
	var clientname = $("#clientname");
	clientname.removeClass("viewer editor director");
	switch (newRank) {
		case Rank.Director:
			clientname.text(myName + " (Director)");
		    clientname.addClass("director");
		    break;
		case Rank.Editor:
			clientname.text(myName + " (Editor)");
		    clientname.addClass("editor");
		    break;
		case Rank.Viewer:
			clientname.addClass("viewer");
		default:
			clientname.text(myName);
	}
	if (newRank >= Rank.Editor) $(".editor_only").show();
	else                        $(".editor_only").hide();
	if (newRank >= Rank.Director) $(".director_only").show();
	else                          $(".director_only").hide();
}

function startNewSession(leader: boolean): void {
	var name = $("#new_client_name").val();
	if (name.length > 0) {
		myName = name;
		if (leader) setMyRank(Rank.Director);
		socket.send(JSON.stringify(buildMessage("SessionInit", {
			Name: name,
			Leader: leader
		})));
	} else {
		alert("You must enter a name for your client.");
	}
}

function sendInvitation(e: Event): void {

}

function selectNewVideo(e: Event): void {
	e.preventDefault();
	if (ytplayer) {
		if (myRank >= Rank.Editor) {
			var newVideoId = $("#video").val();
			if (newVideoId.length != 11) {
				alert("Invalid video ID '" + newVideoId + "'.");
			}
			ytplayer.loadVideoById(newVideoId);
		}
	} else {
		alert("YouTube player is not ready.");
	}
}

function onClientTimeout(): void {
	alert("Lost connection with WeTube client.");
	window.clearInterval(heartbeat);
	socket.close();
}

function elementFromPeer(peer: Peer, parent: JQuery): void {

}

function handleMessage(message: BrowserMessage): void {
	switch (message.Type) {
		case "SessionOk":
			$("#session_type_dialog").hide();
			if (myRank == Rank.Director) {
				initYouTube(videoId);
			} else {
				$("#wait_for_invitation_dialog").show();
			}
			break;
		case "HeartbeatAck":
			window.clearTimeout(timeout);
            timeout = window.setTimeout(onClientTimeout, 10000);
			break;
		case "VideoUpdate":
		
			break;
		case "RosterUpdate":
			var rostermsg = JSON.parse(message.Message);
			var peerlist = $("#peerlist");
			peerlist.empty();
			elementFromPeer(rostermsg.Leader, peerlist);
			$.each(rostermsg.Others, (peer) => elementFromPeer(peer, peerlist));
			break;
		case "EndSession":
			alert("Your WeTube session has ended.");
			window.clearInterval(heartbeat);
			window.clearTimeout(timeout);
			break;
			socket.close();
		case "Error":
			var errmsg = JSON.parse(message.Message).Message;
			console.error("Remote error: " + errmsg);
			break;
		default:
			console.error("Don't know how to handle message of type " + message.Type);
			break;
	}
}

$(document).ready(() => {
	setMyRank(Rank.Unknown);
    socket = new WebSocket('wss://' + window.location.host + '/browserSocket');
    socket.onmessage = (event) => {
    	var message = <BrowserMessage>JSON.parse(event.data);
    	if (message.Type == "BrowserConnectAck") {
            var ack = JSON.parse(message.Message);
            if (ack.Success) {
                console.log("Successfully connected. Client ID: " + ack.Id);
                myId = ack.Id;
                
                heartbeat = window.setInterval(() => {
                    console.log("Sending heartbeat.");
                    socket.send(JSON.stringify(buildMessage("Heartbeat", {Random: Math.round(Math.random()*100000)})));
                }, 5000)
                timeout = window.setTimeout(onClientTimeout, 10000);
                
            	socket.onmessage = (event) => handleMessage(<BrowserMessage>JSON.parse(event.data));
    			socket.onerror = (error) => {
    				alert("WebSocket error occurred. Connection lost.");
					window.clearInterval(heartbeat);
					window.clearTimeout(timeout);
    			}
    			
    			$("#session_type_dialog").show();
    		} else {
    			alert("Failed to connect to client: " + ack.Reason);
    		}
        } else {
            console.error("Got something other than a BrowserConnectAck from client.");
        }
    }
    $("#new_session_button").on('click', () => startNewSession(true));
    $("#join_session_button").on('click', () => startNewSession(false));
    $("#video_form").on('submit', selectNewVideo);
    $("#invite_button").on('click', () => $("#invitation_dialog").show());
    $("#invitation_form").on('submit', sendInvitation);
    $("#invitation_dialog_cancel_button").on('click', () => $("#invitation_dialog").hide());
    $("#peer_info_dialog_close_button").on('click', () => $("#peer_info_dialog").hide());
});
