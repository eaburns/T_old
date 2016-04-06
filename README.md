[![Build Status](https://travis-ci.org/eaburns/T.svg?branch=master)](https://travis-ci.org/eaburns/T)
[![Coverage Status](https://coveralls.io/repos/eaburns/T/badge.svg?branch=master&service=github)](https://coveralls.io/github/eaburns/T?branch=master)
[![GoDoc](https://godoc.org/github.com/eaburns/T?status.svg)](https://godoc.org/github.com/eaburns/T)

T is a text editor
inspired by the Acme and Sam
editors of the [Plan9](http://plan9.bell-labs.com/plan9/) operating system
and [Plan9 from User Space](https://swtch.com/plan9port/) project.

T is still in the early stages of development.
At the moment,
the most useful portion of T is the edit library.
It implements a dialect of the Sam language.
This language is used for editing
buffers of Unicode characters
which, like the Go programming language in which it is written,
T calls runes.
See https://godoc.org/github.com/eaburns/T/edit for more info.

In the future,
T will use this library
as the backend for an editor
much like Acme.
For a taste of Acme,
see [Russ Cox's tour](http://research.swtch.com/acme).

Here's a screenshot of the latest UI prototype:
![screenshot](https://raw.githubusercontent.com/wiki/eaburns/T/screenshot.png)
This prototypes simple, tile-based window management.
Dispite the screenshot showing text,
the UI does not yet have the ability to edit text.

[This document](https://docs.google.com/document/d/1a6HoqavYRvn6OWaxeg4PGVEbnxM0VYm_ekjishCnM8s/edit?usp=sharing)
details some thoughts about the client/server aspect of T.
They are just thoughts and are subject to change.
