[![Build Status](https://travis-ci.org/eaburns/T.svg?branch=master)](https://travis-ci.org/eaburns/T)
[![Coverage Status](https://coveralls.io/repos/eaburns/T/badge.svg?branch=master&service=github)](https://coveralls.io/github/eaburns/T?branch=master)
[![GoDoc](https://godoc.org/github.com/eaburns/T?status.svg)](https://godoc.org/github.com/eaburns/T)

T is a text editor inspired by the Acme and Sam editors
of the [Plan9](http://plan9.bell-labs.com/plan9/) operating system
and [Plan9 from User Space](https://swtch.com/plan9port/) project.
It aims to be much like Acme (see [Russ Cox's tour of Acme for a taste](http://research.swtch.com/acme)).


T is still in the early stages of development.
Here's a screenshot of the latest demo:
![screenshot](https://raw.githubusercontent.com/wiki/eaburns/T/screenshot.png)
You can try it yourself with a couple of simple commands:
```
go get -u github.com/eaburns/T/...
go run $GOPATH/src/github.com/eaburns/T/ui/main.go
```

By design, the T editor is client/server based
It serves an HTTP API allowing client programs to implement editor extensions.

The API is split across two different servers.
One is the editor backend server which actually performs edits on buffers using the T edit language.
The editor server supports multiple buffers and multiple, concurrent editors per-buffer.
This allows programs to easily edit files using a simple HTTP interface.

One such program is the T user interface itself.
The user interface connects to edit servers to provide an Acme-like interface for editing.
All edits made by the user interface use the editor API.
This ensures that the editor API is sufficient to support a full-featured graphical editor,
and it also allows the interface to sit on a remote machine.

The user interface is also the second HTTP server.
It serves an API for manipulating the interface, creating new windows, columns, sheets, etc.

Together, these two servers will make it possible to customize and extend T to your liking —
assuming that you are happy writing a little bit of code.
