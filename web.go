package main

import (
	"fmt"
	"log"
	"net/http"
	"io/ioutil"
)

func ServeWeb() {
	http.HandleFunc("/search/", searchHandler)
	http.HandleFunc("/", frontpageHandler)
	log.Println("Starting webserver.")
	http.ListenAndServe(":9048", nil)
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, r)
}

func frontpageHandler(w http.ResponseWriter, r *http.Request) {
	//add the <html> element
	fmt.Fprint(w, "<html>")
	
	//add the <head> element
	fmt.Fprint(w, "<head>")
	
	//add the stylesheet
	fmt.Fprint(w, "<style type=\"text/css\">")
	file, err := ioutil.ReadFile("webui/style.css")
		if err != nil { panic(err) }
	fmt.Fprint(w, string(file))
	fmt.Fprint(w, "</style>")

	//add the javascript file
	fmt.Fprint(w, "<script type=\"text/javascript\">")
	file, err = ioutil.ReadFile("webui/common.js")
		if err != nil { panic(err) }
	fmt.Fprint(w, string(file))
	fmt.Fprint(w, "</script>")
		
	//close the <head> element
	fmt.Fprint(w, "</head>")
	
	//add the <body> element
	fmt.Fprint(w, "<body>")
	
	//add the name that hovers above the search bar
	fmt.Fprint(w, "<div class = \"name\">Distru</div>")
	
	//add the form
	fmt.Fprint(w, "<form action=\"/search/\" method=\"post\" name=\"search\">")
	fmt.Fprint(w, "<input type=\"text\" id=\"search\" class=\"search\" name=\"search\" placeholder=\"Search freely\"/>")
	fmt.Fprint(w, "<input type=\"submit\" value=\"Submit\" name=\"search\" class=\"hidden\" onclick=\"searchThis()\"/>")
	fmt.Fprint(w, "</form>")
	
	//close the <body> element
	fmt.Fprint(w, "</body>")

	//close the <html> element
	fmt.Fprint(w, "</html>")
	
}
