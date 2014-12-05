package main

import (
	"log"
	"net/http"
)

func main() {
	log.Println("Starting WeTube HTTP server on localhost:9191...")
	log.Fatal(http.ListenAndServe(":9191", http.FileServer(http.Dir("./web/"))))
}
