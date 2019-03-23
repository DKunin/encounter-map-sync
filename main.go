package main

import (
	"fmt"
	"github.com/boltdb/bolt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/lonng/nano"
	"github.com/lonng/nano/component"
	"github.com/lonng/nano/serialize/json"
	"github.com/lonng/nano/session"
	"strings"
)

type (
	Room struct {
		group *nano.Group
	}

	// RoomManager represents a component that contains a bundle of room
	RoomManager struct {
		component.Base
		timer *nano.Timer
		rooms map[int]*Room
	}

	// UserMessage represents a message that user sent
	UserMessage struct {
		X                  string `json:"x"`
		Y                  string `json:"y"`
		Id                 string `json:"id"`
		IsCurrentlyVisible string `json:"isCurrentlyVisible"`
	}

	// NewUser message will be received when new user join room
	NewUser struct {
		Content       string `json:"content"`
		EncounterData string `json:"encounter"`
	}

	TypeDeclaration struct {
		Type      string `json:"type"`
		Encounter string `json:"encounter"`
	}

	// AllMembers contains all members uid
	AllMembers struct {
		Members []int64 `json:"members"`
	}

	// JoinResponse represents the result of joining room
	JoinResponse struct {
		Code   int    `json:"code"`
		Result string `json:"result"`
	}

	stats struct {
		component.Base
		timer         *nano.Timer
		outboundBytes int
		inboundBytes  int
	}

	Server struct {
		db    *bolt.DB
	}

)

func (stats *stats) outbound(s *session.Session, msg nano.Message) error {
	stats.outboundBytes += len(msg.Data)
	return nil
}

func (stats *stats) inbound(s *session.Session, msg nano.Message) error {
	stats.inboundBytes += len(msg.Data)
	return nil
}

func (stats *stats) AfterInit() {
	stats.timer = nano.NewTimer(time.Minute, func() {
		//println("OutboundBytes", stats.outboundBytes)
		//println("InboundBytes", stats.outboundBytes)
	})
}

const (
	RoomId    = 1
	roomIDKey = "ROOM_ID"
)

func NewRoomManager() *RoomManager {
	return &RoomManager{
		rooms: map[int]*Room{},
	}
}

// AfterInit component lifetime callback
func (mgr *RoomManager) AfterInit() {
	session.Lifetime.OnClosed(func(s *session.Session) {
		if !s.HasKey(roomIDKey) {
			return
		}
		room := s.Value(roomIDKey).(*Room)
		room.group.Leave(s)
	})
	mgr.timer = nano.NewTimer(time.Minute, func() {
		for roomId, room := range mgr.rooms {
			println(fmt.Sprintf("UserCount: RoomID=%d, Time=%s, Count=%d",
				roomId, time.Now().String(), room.group.Count()))
		}
	})
}

// Join room
func (mgr *RoomManager) Join(s *session.Session, msg []byte) error {
	// NOTE: join test room only in demo
	room, found := mgr.rooms[RoomId]
	if !found {
		room = &Room{
			group: nano.NewGroup(fmt.Sprintf("room-%d", RoomId)),
		}
		mgr.rooms[RoomId] = room
	}
	fakeUID := s.ID() //just use s.ID as uid !!!
	s.Bind(fakeUID)   // binding session uids.Set(roomIDKey, room)
	s.Set(roomIDKey, room)
	s.Push("onMembers", &AllMembers{Members: room.group.Members()})
	// notify others
	room.group.Broadcast("onNewUser", &NewUser{EncounterData: fmt.Sprintf("%s", msg), Content: fmt.Sprintf("New user: %d", s.ID())})
	// new user join group
	room.group.Add(s) // add session to group
	return s.Response(&JoinResponse{Result: "success"})
}


type Data struct {
	Info string `json:"data"`
}

// Message sync last message to all members
func (mgr *RoomManager) Message(s *session.Session, msg *UserMessage) error {
	if !s.HasKey(roomIDKey) {
		return fmt.Errorf("not join room yet")
	}
	room := s.Value(roomIDKey).(*Room)
	return room.group.Broadcast("onMessage", msg)
}

func SettingsHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			err := r.ParseForm()
			if err != nil {
				w.WriteHeader(500)
				log.Printf("error with form %s", err)
				return
			}

			post := Data{
				Info:  r.PostFormValue("data"),
			}

			jsonData, err := json.Marshal(post)
			jsonFile, err := os.Create("./Person.json")

			if err != nil {
				panic(err)
			}
			defer jsonFile.Close()

			jsonFile.Write(jsonData)
			jsonFile.Close()
			fmt.Println("JSON data written to ", jsonFile.Name())

			//Server.db.Update(func(tx *bolt.Tx) error {
			//	b, _ := tx.CreateBucketIfNotExists([]byte("Posts"))
			//	id, _ := b.NextSequence()
			//	j, _ := json.Marshal(post)
			//	log.Printf("json: %s", j)
			//	err := b.Put([]byte(strconv.Itoa(int(id))), j)
			//	if err != nil {
			//		log.Printf("broke wrote to db %v", err)
			//	}
			//	return err
			//})

			w.WriteHeader(200)
			w.Write([]byte("asd")
			return
		}
		//
		//b, err := ioutil.ReadFile("assets/settings.html")
		//
		//if err != nil {
		//	w.WriteHeader(500)
		//	w.Write([]byte("Error reading file"))
		//	log.Fatal(err)
		//	return
		//}
		w.WriteHeader(200)
		w.Write([]byte("b"))
	}
}

func main() {
	// override default serializer
	nano.SetSerializer(json.NewSerializer())
	db, err := bolt.Open("data.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	// rewrite component and handler name
	room := NewRoomManager()
	nano.Register(room,
		component.WithName("room"),
		component.WithNameFunc(strings.ToLower),
	)

	// traffic stats
	pipeline := nano.NewPipeline()
	var stats = &stats{}
	pipeline.Outbound().PushBack(stats.outbound)
	pipeline.Inbound().PushBack(stats.inbound)

	nano.EnableDebug()
	log.SetFlags(log.LstdFlags | log.Llongfile)
	nano.SetWSPath("/nano")

	http.Handle("/web/", http.StripPrefix("/web/", http.FileServer(http.Dir("web"))))
	http.HandleFunc("/settings", SettingsHandler(db))

	nano.SetCheckOriginFunc(func(_ *http.Request) bool { return true })
	nano.ListenWS(":3250", nano.WithPipeline(pipeline))
}
