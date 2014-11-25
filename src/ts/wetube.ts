/// <reference path="typings/jquery.d.ts" />
/// <reference path="typings/swfobject.d.ts" />
/// <reference path="typings/youtube.d.ts" />

function initYouTube(videoId: string): void {
    var params = { allowScriptAccess: 'always', allowFullScreen: 'true' };
    var attrs = { id: 'ytapiplayer' };
    swfobject.embedSWF("http://www.youtube.com/v/" + videoId +
        "?hl=en_US&rel=0&hd=1&border=0&version=3&fs=1&autoplay=1&autohide=0&enablejsapi=1&playerapiid=ytapiplayer",
        'ytapiplayer', '640', '360', '8', null, null, params, attrs);
}

function onYouTubePlayerReady(id) {
    console.log("YouTube player is ready.");
}

$(document).ready(() => initYouTube("dQw4w9WgXcQ")); // Never gonna give you up...
