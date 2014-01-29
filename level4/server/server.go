package server

import (
	"github.com/gorilla/mux"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"stripe-ctf.com/sqlcluster/log"
	"stripe-ctf.com/sqlcluster/sql"
	"stripe-ctf.com/sqlcluster/transport"
    "stripe-ctf.com/sqlcluster/util"
	"stripe-ctf.com/sqlcluster/command"
    "github.com/goraft/raft"
	"time"
	"fmt"
	"math/rand"
	"strings"
)

type Server struct {
	name       string
	path       string
	listen     string
	router     *mux.Router
	httpServer *http.Server
	sql        *sql.SQL
	client     *transport.Client
	cluster    *Cluster
    raftServer raft.Server
    leader    string
}

type Join struct {
	Self ServerAddress `json:"self"`
}

type JoinResponse struct {
	Self    string   `json:"self"`
	Members []string `json:"members"`
}

type Replicate struct {
	Self  ServerAddress `json:"self"`
	Query []byte        `json:"query"`
}

type ReplicateResponse struct {
	Self ServerAddress `json:"self"`
}

// Creates a new server.
func New(path, listen string) (*Server, error) {
	cs, err := transport.Encode(listen)
	if err != nil {
		return nil, err
	}

	sqlPath := filepath.Join(path, "storage.sql")
	util.EnsureAbsent(sqlPath)

	s := &Server{
		path:    path,
		listen:  listen,
		sql:     sql.NewSQL(sqlPath),
		router:  mux.NewRouter(),
		client:  transport.NewClient(),
		cluster: NewCluster(path, cs),
	}


	// Read existing name or generate a new one.
    if b, err := ioutil.ReadFile(filepath.Join(path, "name")); err == nil {
            s.name = string(b)
    } else {
            s.name = fmt.Sprintf("%07x", rand.Int())[0:7]
            if err = ioutil.WriteFile(filepath.Join(path, "name"), []byte(s.name), 0644); err != nil {
                    panic(err)
            }
    }



	return s, nil
}

// Returns the connection string.
func (s *Server) connectionString() string {
    cs, err := transport.Encode(s.listen)
    if err != nil {
		log.Fatal(err)
    }
    return cs
}


// Starts the server.
func (s *Server) ListenAndServe(primary string) error {
    var err error
    // Initialize and start Raft server.
    transporter := raft.NewHTTPTransporter("/raft")
    s.raftServer, err = raft.NewServer(s.name, s.path, transporter, nil, s.sql, s.connectionString())
    if err != nil {
		log.Fatal(err)
    }

    s.leader = primary
    
    transporter.Install(s.raftServer, s)
    s.raftServer.Start()


    if primary != "" {
        // Join to primary if specified.

        log.Println("Attempting to join primary:", primary)

        if !s.raftServer.IsLogEmpty() {
                log.Fatal("Cannot join with an existing log")
        }
        if err := s.Join(primary); err != nil {
                log.Fatal(err)
        }

    } else if s.raftServer.IsLogEmpty() {
            // Initialize the server by joining itself.

            log.Println("Initializing new cluster")

            _, err := s.raftServer.Do(&raft.DefaultJoinCommand{
                    Name:             s.raftServer.Name(),
                    ConnectionString: s.connectionString(),
            })
            if err != nil {
                    log.Fatal(err)
            }

    } else {
            log.Println("Recovered from log")
    }

    log.Println("Initializing HTTP server")

    // Initialize and start HTTP server.
    s.httpServer = &http.Server{
        Handler: s.router,
    }

	s.router.HandleFunc("/sql", s.sqlHandler).Methods("POST")
	s.router.HandleFunc("/pqr", s.sqlXHandler).Methods("POST")
	s.router.HandleFunc("/healthcheck", s.healthcheckHandler).Methods("GET")
	s.router.HandleFunc("/join", s.joinHandler).Methods("POST")

	// Start Unix transport
	l, err := transport.Listen(s.listen)
	if err != nil {
		log.Fatal(err)
	}

	return s.httpServer.Serve(l)
}

// Join an existing cluster
func (s *Server) Join(leader string) error {


	command := &raft.DefaultJoinCommand{
            Name:             s.raftServer.Name(),
            ConnectionString: s.connectionString(),
    }
	b := util.JSONEncode(command)

	cs, err := transport.Encode(leader)
	if err != nil {
		return err
	}

	for {
		body, err := s.client.SafePost(cs, "/join", b)
		if err != nil {
			log.Printf("Unable to join cluster: %s", err)
			time.Sleep(1 * time.Second)
			continue
		}

		resp := &JoinResponse{}
		if err = util.JSONDecode(body, &resp); err != nil {
			return err
		}

		return nil
	}
}

func (s *Server) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
        s.router.HandleFunc(pattern, handler)
}

// Server handlers
func (s *Server) joinHandler(w http.ResponseWriter, req *http.Request) {
	log.Println("Recieved a call to JOIN")

	j := &raft.DefaultJoinCommand{}
	if err := util.JSONDecode(req.Body, j); err != nil {
		log.Printf("Invalid join request: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Handling join request: %#v", j)

	if _, err := s.raftServer.Do(j); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    peers := s.raftServer.Peers()
    mk := make([]string, len(peers))
    i := 0
    for k, _ := range peers {
        mk[i] = k
        i++
    }

    resp := &JoinResponse{
		s.raftServer.Name(),
		mk,
	}
	b := util.JSONEncode(resp)

	log.Println("JOINED")
	w.Write(b.Bytes())
}

// This is the only user-facing function, and accordingly the body is
// a raw string rather than JSON.
func (s *Server) sqlHandler(w http.ResponseWriter, req *http.Request) {	

	query, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Printf("Couldn't read body: %s", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	
	log.Println("Talking to ", s.raftServer.State())
	log.Println("Recieved a call to SQL ")

	state := s.raftServer.State()	
	if state != "leader" {
		log.Println("\n\nFORWARDING\n\n")
		fresp := s.forwardQuery(string(query))
		if fresp == nil {
			http.Error(w, "Only the primary can service queries, but this is a "+state, http.StatusBadRequest)
		} else {
			w.Write(fresp)
		}
		return
	}


    resp, err := s.raftServer.Do(&command.QueryCommand{
			Query: string(query),
		})
    log.Println("response is ", resp)
    if err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
    }


	w.Write(resp.([]byte))
}


// This is the only user-facing function, and accordingly the body is
// a raw string rather than JSON.
func (s *Server) sqlXHandler(w http.ResponseWriter, req *http.Request) {	

	query, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Printf("Couldn't read body: %s", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	
	log.Println("Talking to X ", s.raftServer.State())

	state := s.raftServer.State()
	log.Printf("RAFT SERVER %s foo", s.path)
	if state != "leader" {				
		http.Error(w, "Only the primary can service queries, but this is a "+state, http.StatusBadRequest)
		return
	}

	log.Println("Recieved a call to SQL ")

    resp, err := s.raftServer.Do(&command.QueryCommand{
			Query: string(query),
		})
    log.Println("response is ", resp)
    if err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
    }


	w.Write(resp.([]byte))
}

func (s *Server) forwardQuery(query string) []byte {
	b := strings.NewReader(query)
	for i := 0; i < rand.Intn(5); i++ {

		log.Println("\n\nTrying No. ", i)
		
		cs, _ := transport.Encode(fmt.Sprintf("./node%d.sock", i))

		body, err := s.client.SafePost(cs, "/pqr", b)
		if err == nil {
			result, _ := ioutil.ReadAll(body)
			return result
		}
	}
	return nil
}

func (s *Server) healthcheckHandler(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
}