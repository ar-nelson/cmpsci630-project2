WeTube - CMPSCI 630 Project #2
==============================

A JavaScript web application, powered by a Go server application, that
synchronizes a YouTube video between multiple viewers using a peer-to-peer
network.

Compiling and Running
---------------------

WeTube uses [Grunt][grunt] as its build system. To compile and run it, perform
the following steps **(Warning: only tested on Windows!)**:

1. Install [node.js][nodejs] and [npm][npm].

2. Run `npm install -g grunt-cli`

3. Navigate to WeTube's root directory, and run `npm install`.

4. Run `grunt`. This will compile and launch the WeTube client, and will also
   open a browser window after a short delay.
   
5. You may need to add an HTTPS certificate exception in the browser, because
   the certificate is self-signed; if the client times out while you do this,
   close the browser window and try running `grunt` again.

[grunt]: http://gruntjs.com/
[nodejs]: http://nodejs.org/
[npm]: http://npmjs.org/

Usage
-----

When the WeTube application starts, you can select *Start New Session* or *Join
Existing Session*.

### Start New Session

You will be immediately taken to the main WeTube interface. The *Invite New
Viewer* button allows you to send invitations to clients who chose the other
option (*Join Existing Session*). You should not need to change the *Port*
number; all WeTube instances launched from Grunt use port 9191.

### Join Existing Session

The client will wait for a session leader to send an invitation. When an
invitation is received, you will be told what video the session is viewing, and
be given the option to accept or reject the invitation.

If you join a session as a Viewer (rather than an Editor or Director), you will
not have permission to seek or change the video. Although it is not possible to
remove the controls from the video, if you use them and your video becomes out
of sync with the rest of the session, the video will be adjusted back to its
previous position the next time an update is received from the leader.

Internals
---------

All communication in WeTube--even between Go client peers--is done via
WebSockets. Connections between two peers use two WebSocket connections, one for
each direction. This was done for simplicity; WebSockets abstract over some of
the complexity involved in managing multiple TCP connections, by letting the
HTTP server manage the connections.

The client-to-browser WebSocket `/browserSocket` sends messages in JSON, while
the client-to-client WebSocket `/peerSocket` sends messages in Go's binary
[gob][gob] format. Both sockets use the same object protocol.

All HTTP and WS communication is encrypted with TLS. In addition,
client-to-client messages are digitally signed using RSA, and these signatures
are verified whenever a message is received (except during the initial
invitation handshake).

All connections (client-to-browser and client-to-client) send a heartbeat signal
every 5 seconds, and time out if this signal is not received after 10 seconds.
While the WebSocket protocol itself will usually report an error in most
disconnection events, the heartbeat system is a more reliable way to guarantee
that a connection is closed even in the event of an unexpected hang or deadlock.

[gob]: http://blog.golang.org/gobs-of-data

