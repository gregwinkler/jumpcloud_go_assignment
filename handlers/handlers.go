package handlers

import (
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

// HashResult -
// track value and duration for stats
type HashResult struct {
	duration int64
	hash     string
}

// yes, 'global' variables bad... but I'm still getting the hang of this
// write access to this map should also be guarded by a mutex... multiple
// threads/responses are potentially writing to it.
var (
	shuttingDown     bool
	requestResponses map[string]HashResult
	ShutdownChan     chan bool
)

// GetHash - 5 seconds after the POST to /hash that returned an identifer you should be able to curl
// <localhost:port>/hash/42 and get back the value of
// “ZEHhWB65gUlzdVwtDQArEyx+KVLzp/aTaRaPlBzYRIFj6vjFdqEb0Q5B8zVKCZ0vKbZPZklJz0Fd7su2A+gf7Q==”.
func GetHash(w http.ResponseWriter, r *http.Request) {
	// make sure
	if shuttingDown {
		w.WriteHeader(http.StatusServiceUnavailable) // error code not specified in specs. using internets...
		fmt.Fprintf(w, "server shutting down")
		return
	}

	// Double check it's a post request being made
	if r.Method == http.MethodGet {
		vars := mux.Vars(r)
		id := vars["id"]

		if val, ok := requestResponses[id]; !ok { // doesn't exist
			w.WriteHeader(http.StatusNotFound) // 404 - this resource does not exist
		} else {
			// not wholly confortable with OK and blank here...
			// would prefer a 'still pending' (I tried StatusPending, but that wasn't it)
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "%s", val.hash)
		}
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "invalid http method")
	}
}

// HandleIndex returns a json welcome msg to the user
func HandleIndex(w http.ResponseWriter, r *http.Request) {
	// make sure
	if shuttingDown {
		w.WriteHeader(http.StatusServiceUnavailable) // error code not specified in specs. using internets...
		fmt.Fprintf(w, "server shutting down")
		return
	}

	w.Header().Set("Content-type", "application/json")
	fmt.Fprintf(w, "{ \"msg\" : \"Hello, this is the index page of Greg Winkler's coding assignment in Go\" }")
}

// ProcessHash - Accept POST requests on the /hash endpoint with a form field named password to provide
// the value to hash. An incrementing identifier should return immediately but the password should
// not be hashed for 5 seconds. The hash should be computed as base64 encoded string of the
// SHA512 hash of the provided password.
func ProcessHash(w http.ResponseWriter, r *http.Request) {

	// make sure
	if shuttingDown {
		w.WriteHeader(http.StatusServiceUnavailable) // error code not specified in specs. using internets...
		fmt.Fprintf(w, "server shutting down")
		return
	}

	// Double check it's a post request being made
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "invalid http method")
		return
	}
	// Must call ParseForm() before working with data
	r.ParseForm()

	var pwd = r.Form.Get("password")

	// error check
	if pwd == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprintf(w, "Missing password argument")
		return
	}

	// this is one place where multithreaded might bite us...
	// multiple threads/requests could be writing at this point. Getting the request id
	// up front limits potential damage, but isn't foolproof
	var req = strconv.Itoa(len(requestResponses) + 1)
	requestResponses[req] = HashResult{duration: -1, hash: ""}

	go hashPassword(req, pwd) // async call to hash the password 5 seconds from now

	fmt.Fprintf(w, req)
}

// hashPassword -
// async call to hash the password at this index and fill that item
// in the cache
func hashPassword(index string, pwd string) {
	// per instructions, wait 5000ms to make the hash available to
	// caller thru get. Once hashed, send it back to the
	// channel to fill in the requestResponses

	// the instructions are a little unclear as to what I'm timing... the
	// WHOLE execution including the wait, or just the hashing exec.
	// Given the numbers I'm seeing (always less than 0 ms) for the hashing, I'm going with the 'whole' processing
	start := time.Now()

	time.Sleep(5 * time.Second)

	h := sha512.New()
	h.Write([]byte(pwd))
	hash := base64.URLEncoding.EncodeToString(h.Sum(nil))
	duration := time.Since(start)

	requestResponses[index] = HashResult{hash: hash, duration: duration.Milliseconds()}

	if shuttingDown {
		if !pendingRequestsExist() {
			ShutdownChan <- true
			close(ShutdownChan) // to ask - is this correct? Not sure if it's <- true or close()
		}
	}
}

// pendingRequestsExist -
// return true if there's still a hash waiting (a hash that doesn't have a hash :)
// return false otherwise
func pendingRequestsExist() bool {
	for _, v := range requestResponses {
		if v.hash == "" {
			return true
		}
	}

	return false
}

// InitializeRoutes - create the router you return to the caller and setup the
// paths to the handlers in here
func InitializeRoutes() *mux.Router {
	requestResponses = make(map[string]HashResult)

	ShutdownChan = make(chan bool, 1)

	router := mux.NewRouter()

	router.HandleFunc("/hash/{id}", GetHash).Methods("GET")
	router.HandleFunc("/hash", ProcessHash).Methods("POST")
	router.HandleFunc("/stats", GetStats).Methods("GET")
	router.HandleFunc("/shutdown", Shutdown).Methods("GET")
	router.HandleFunc("/", HandleIndex).Methods("GET")

	return router
}

// GetStats -
//
// A GET request to /stats should return a JSON object with 2 key/value pairs. The “total”
// key should have a value for the count of POST requests to the /hash endpoint made to the
// server so far. The “average” key should have a value for the average time it has taken to
// process all of those requests in microseconds.
func GetStats(w http.ResponseWriter, r *http.Request) {
	// make sure -- could have made this check part of our own handler function
	if shuttingDown {
		w.WriteHeader(http.StatusServiceUnavailable) // error code not specified in specs. using internets...
		fmt.Fprintf(w, "server shutting down")
		return
	}

	// Double check it's a post request being made
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "invalid http method")
		return
	}

	sum := int64(0)
	num := int64(0)
	for _, v := range requestResponses {
		if v.hash != "" {
			num++
			sum += v.duration
		}
	}
	// FUTURE TO DO - serialize object instead of building JSON string
	w.Header().Set("Content-type", "application/json")
	if num == 0 {
		fmt.Fprintf(w, "{ \"total\" : 0, \"average\" : 0 }")
	} else {
		fmt.Fprintf(w, "{ \"total\" : %v, \"average\" : %v }", num, int(sum/num))
	}

}

// Shutdown -
// Provide support for a “graceful shutdown request”. If a request is made to /shutdown the
// server should reject new requests. The program should wait for any pending/in-flight work to
// finish before exiting.
func Shutdown(w http.ResponseWriter, r *http.Request) {

	// Double check it's a post request being made
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "invalid http method")
		return
	}
	ShutdownChan <- true
	shuttingDown = true
}
