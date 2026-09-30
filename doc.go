/*
Command dbcli is a read-only SQL CLI built for AI agents. Every query runs
inside a read-only, timeout-bounded, row-capped transaction, and every
result, success or failure, is a stable JSON envelope on stdout.

Installation:

	go install github.com/diogoaalmeida/dbcli@latest

For the full command list, flags, safety guarantees, and setup
instructions, see the README:
https://github.com/diogoaalmeida/dbcli
*/
package main
