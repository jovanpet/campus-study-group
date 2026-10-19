package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	fmt.Println("Server started on port", port)

	http.HandleFunc("/health", healthHandler)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Health check OK")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
