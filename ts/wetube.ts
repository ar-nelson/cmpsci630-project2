/// <reference path="typings/jquery.d.ts" />
/// <reference path="typings/swfobject.d.ts" />
/// <reference path="typings/youtube.d.ts" />

var socket: WebSocket
var myId: number
var myName: string = "(Client Name)"
var myRank: Rank = Rank.Unknown
var videoId: string = "M7lc1UVf-VE"
var currentInvitation: Invitation = null
var videoHeartbeat: number = null
var heartbeat: number = null
var timeout: number = null
var ytplayer: YT.Player = null
var whenPlayerReady: Function = null
var echoBlock: boolean = false

enum Rank {
    Unknown,
    Viewer,
    Editor,
    Director
}

enum VideoState {
    Unstarted,
    Playing,
    Paused,
    Ended
}

enum YTPlayerState {
    UNSTARTED = -1,
    ENDED     = 0,
    PLAYING   = 1,
    PAUSED    = 2, 
    BUFFERING = 3,
    CUED      = 5
}

interface BrowserMessage {
    Type: string
    Message: string
}

interface PublicKey {
    N: string
    E: number
}

interface Peer {
    Id: number
    Name: string
    Address: string
    Port: number
    Rank: Rank
    PublicKey: PublicKey
}

interface Invitation {
    Leader:           Peer
    RankOffered:      Rank
    CurrentVideoName: string
    Random:           number
}

function initYouTube(vid: string): void {
    var params = { allowScriptAccess: 'always', allowFullScreen: 'true' };
    var attrs = { id: 'ytapiplayer' };
    swfobject.embedSWF("https://www.youtube.com/v/" + vid +
        "?hl=en_US&rel=0&hd=1&border=0&version=3&fs=1&autoplay=0&autohide=0&enablejsapi=1&playerapiid=ytapiplayer",
        'ytapiplayer', '640', '360', '8', null, null, params, attrs);
    videoId = vid
}

function onYouTubePlayerReady(id): void {
    console.log("YouTube player is ready.");
    ytplayer = <YT.Player><any>document.getElementById("ytapiplayer");
    ytplayer.addEventListener('onStateChange', 'sendVideoUpdate');
    if (myRank >= Rank.Editor) {
        videoHeartbeat = window.setInterval(() => sendVideoUpdate(ytplayer.getPlayerState()), 5000)
    }
    if (whenPlayerReady) {
        whenPlayerReady();
        whenPlayerReady = null;
    }
}

function sendVideoUpdate(ytState: number) {
    try {
        var wtState: VideoState
        switch (ytState) {
            case YTPlayerState.PLAYING:
                wtState = VideoState.Playing;
                break;
            case YTPlayerState.BUFFERING:
            case YTPlayerState.PAUSED:
                wtState = VideoState.Paused;
                break;
            case YTPlayerState.ENDED:
                wtState = VideoState.Ended;
                break;
            case YTPlayerState.CUED:
            default:
                wtState = VideoState.Unstarted;
        }
        if (!echoBlock && myRank >= Rank.Editor) {
            socket.send(buildMessage("VideoUpdateRequest", {
                Id: videoId,
                State: wtState,
                SecondsElapsed: ytplayer.getCurrentTime()
            }));
        }
    } catch (ex) {
        console.error(ex)
    }
}

function buildMessage(type: string, contents: any): string {
    return JSON.stringify({
        Type: type,
        Message: JSON.stringify(contents)
    });
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
            clientname.text(myName + " (Viewer)");
            clientname.addClass("viewer");
            break;
        default:
            clientname.text(myName);
    }
    if (newRank >= Rank.Editor) $(".editor_only").show();
    else                        $(".editor_only").hide();
    if (newRank >= Rank.Director) $(".director_only").show();
    else                          $(".director_only").hide();
    if (videoHeartbeat !== null) {
        window.clearInterval(videoHeartbeat);
        videoHeartbeat = null;
    }
    if (ytplayer && newRank >= Rank.Editor) {
        videoHeartbeat = window.setInterval(() =>
            sendVideoUpdate(ytplayer.getPlayerState()), 5000)
    }
}

function startNewSession(leader: boolean): void {
    var name = $("#new_client_name").val();
    if (name.length > 0) {
        myName = name;
        if (leader) setMyRank(Rank.Director);
        socket.send(buildMessage("SessionInit", {
            Name: name,
            Leader: leader
        }));
    } else {
        alert("You must enter a name for your client.");
    }
}

function respondToInvitation(accept: boolean): void {
    $("#invitation_response_dialog").hide();
    if (currentInvitation) {
        $("#wait_for_invitation_dialog").show();
        socket.send(buildMessage("InvitationResponse", {
            Accepted:  accept,
            Id:        myId,
            Name:      myName,
            PublicKey: null,
            Random:    currentInvitation.Random
        }));
    }
}

function sendInvitation(e: Event): void {
    e.preventDefault();
    if (myRank < Rank.Director) return;
    $("#invitation_dialog").hide()
    // Get the video title.
    $.get('https://gdata.youtube.com/feeds/api/videos/' + videoId + '?v=2&alt=json', (data) => {
        var title = data.entry.title.$t;
        socket.send(buildMessage("InvitationRequest", {
            Address: $("#ip").val(),
            Port: parseInt($("#port").val()),
            Invitation: {
                Leader: null,
                RankOffered: parseInt($("#rank").val()),
                CurrentVideoName: title,
                Random: Math.round(Math.random() * 1000000)
            }
        }));
    });
}

function selectNewVideo(e: Event): void {
    e.preventDefault();
    if (ytplayer) {
        if (myRank >= Rank.Editor) {
            var newVideoId = $("#video").val();
            if (newVideoId.length === 11) {
                videoId = newVideoId;
                ytplayer.loadVideoById(newVideoId, 0);
            } else {
                alert("Invalid video ID '" + newVideoId + "'.");
            }
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
    var el = $("<li class='name'></li>");
    switch (peer.Rank) {
        case Rank.Director:
            el.text(peer.Name + " (Director)");
            el.addClass("director");
            break;
        case Rank.Editor:
            el.text(peer.Name + " (Editor)");
            el.addClass("editor");
            break;
        case Rank.Viewer:
            el.text(peer.Name + " (Viewer)");
            el.addClass("viewer");
            break;
        default:
            el.text(peer.Name);
    }
    parent.append(el);
    el.on('click', () => showPeerInfoWindow(peer));
}

function showPeerInfoWindow(peer: Peer): void {
    $("#info_name").text(peer.Name);
    $("#info_id").text(peer.Id);
    $("#info_ip").text(peer.Address);
    $("#info_port").text(peer.Port);
    switch (peer.Rank) {
        case Rank.Director: $("#info_rank").text("Director"); break;
        case Rank.Editor: $("#info_rank").text("Editor"); break;
        case Rank.Viewer: $("#info_rank").text("Viewer"); break;
        default: $("#info_rank").text("Unknown");
    }
    $("#rank_change_form").on('submit', (e) => {
        e.preventDefault();
        socket.send(buildMessage("RankChangeRequest", {
            PeerId: peer.Id,
            NewRank: parseInt($("#newrank").val())
        }));
        $("#peer_info_dialog").hide();
    });
    $("#peer_info_dialog").show();
}

function handleMessage(message: BrowserMessage): void {
    switch (message.Type) {
        case "Invitation":
            $("#wait_for_invitation_dialog").hide();
            if (myRank == Rank.Unknown) {
                currentInvitation = JSON.parse(message.Message);
                $("#invitation_name").text(currentInvitation.Leader.Name);
                $("#invitation_video").text(currentInvitation.CurrentVideoName);
                $("#invitation_response_dialog").show();
            }
            break;
        case "JoinConfirmation":
            if (myRank == Rank.Unknown) {
                var joinmsg = JSON.parse(message.Message);
                if (joinmsg.Success) {
                    setMyRank(currentInvitation.RankOffered);
                    initYouTube(videoId);
                    $("#wait_for_invitation_dialog").hide();
                } else {
                    alert("Join failed: " + joinmsg.Reason);
                }
            }
            break;
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
            var videomsg = JSON.parse(message.Message);
            echoBlock = true;
            window.setTimeout(() => echoBlock = false, 2000);
            if (!ytplayer) {
                videoId = videomsg.Id;
                initYouTube(videomsg.Id);
                whenPlayerReady = () => {
                    ytplayer.seekTo(videomsg.SecondsElapsed, true);
                    switch (videomsg.State) {
                        case VideoState.Paused: ytplayer.pauseVideo(); break;
                        case VideoState.Playing: ytplayer.playVideo(); break;
                        case VideoState.Ended: ytplayer.stopVideo();
                    }
                }
                return;
            }
            
            if (videomsg.Id == videoId) {
                ytplayer.seekTo(videomsg.SecondsElapsed, true);
            } else {
                videoId = videomsg.Id;
                ytplayer.loadVideoById(videoId, videomsg.SecondsElapsed);
            }
            switch (videomsg.State) {
                case VideoState.Unstarted:
                    if (ytplayer.getPlayerState() == YTPlayerState.PLAYING) {
                        ytplayer.pauseVideo();
                    }
                    break;
                case VideoState.Paused:
                    if (ytplayer.getPlayerState() != YTPlayerState.PAUSED) {
                        ytplayer.pauseVideo();
                    }
                    break;
                case VideoState.Playing:
                    if (ytplayer.getPlayerState() != YTPlayerState.PLAYING) {
                        ytplayer.playVideo();
                    }
                    break;
                case VideoState.Ended:
                    ytplayer.stopVideo();
                    break;
            }
            break;
        case "RosterUpdate":
            var rostermsg = JSON.parse(message.Message);
            var peerlist = $("#peerlist");
            peerlist.empty();
            for (var i=0; i<rostermsg.Roster.length; i++) {
                if (rostermsg.Roster[i].Id == myId) {
                    if (rostermsg.Roster[i].Rank != myRank) {
                        setMyRank(rostermsg.Roster[i].Rank);
                    }
                } else {
                    elementFromPeer(rostermsg.Roster[i], peerlist);
                }
            }
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
                    socket.send(buildMessage("Heartbeat", {
                        Random: Math.round(Math.random() * 1000000)
                    }));
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
    $("#accept_invitation_button").on('click', () => respondToInvitation(true));
    $("#reject_invitation_button").on('click', () => respondToInvitation(false));
    $("#video_form").on('submit', selectNewVideo);
    $("#invite_button").on('click', () => $("#invitation_dialog").show());
    $("#end_session_button").on('click', () => {
        if (ytplayer) ytplayer.stopVideo()
        socket.send(buildMessage("EndSession", {LeaderId: myId}));
    });
    $("#invitation_form").on('submit', sendInvitation);
    $("#invitation_dialog_cancel_button").on('click', (e) => {
        e.preventDefault();
        $("#invitation_dialog").hide();
    });
    $("#peer_info_dialog_close_button").on('click', () => $("#peer_info_dialog").hide());
});
