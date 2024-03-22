package main

import(
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
	"context"
	"os"
	"os/signal"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/thedevsaddam/renderer"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)


var rnd *renderer.Render // renderer.Render is a struct that implements the chi.Renderer interface
var db *mgo.Database // mgo.Database 

//constants that will be used in the project
const(
	hostName    string = "localhost27017"
	dbName      string = "go-todo"
	collectionName  string = "todo"
	port string = "9000"
)

type(
	//bson data we get from the database
	todoModel struct {
		ID bson.ObjectId `bson:"_id,omitempty"`
		Title string `json:"title"`
		Completed bool `bson:"completed"`
		CreatedAt time.Time `bson:"created_at"`
	}
	// data we send to the client
	todo struct {
		ID string `json:"id"`
		Title string `json:"title"`
		Completed bool `json:"completed"`
		CreatedAt string `json:"created_at"`
}
)	

func init(){
	rnd = renderer.New()
	sess, err := mgo.Dial(hostName)
	checkErr(err)
	sess.SetMode(mgo.Monotonic, true)
	db = sess.DB(dbName)
}

// w is the response writer, r is the request
func homeHandler(w http.ResponseWriter, r *http.Request){
	err:= rnd.Template(w, http.StatusOK, []string{"static/home.tpl"}, nil)
	checkErr(err)
}

func fetchTodos(w http.ResponseWriter, r *http.Request){
	todos := [] todoModel{}
	if err:= db.C(collectionName).Find(bson.M{}).All(&todos); err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Error fetching todos",
			"error": err,

		})
		return	
	}
	// populating the todo list
	todoList := [] todo {}

	// gets both the iterator and the value but since iterator is not used we keep _ to avoid error
	for _, t := range todos {
		todoList = append (todoList,todo{
			ID: t.ID.Hex(),
			Title: t.Title,
			Completed: t.Completed,
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
		})
	}
	// rendering the list of todos
	rnd.JSON(w,http.StatusOK, renderer.M{
		"data": todoList,
	})
}

func createTodo(w http.ResponseWriter, r *http.Request){
	var t todo

	if err:= json.NewDecoder(r.Body).Decode(&t); err!=nil{
		// returning the error to the client
		rnd.JSON(w,http.StatusProcessing, err)
		return
	}
	// checking if the title is empty
	if t.Title == "" {
		rnd.JSON(w,http.StatusProcessing, renderer.M{
			"message": "Title is required",
		})
		return
	
	}

	// creating a new todo model to send it to the database hence bson
	tm := todoModel{
		ID: bson.NewObjectId(),
		Title: t.Title,
		Completed: false,
		CreatedAt: time.Now(),
	}

	if err:=db.C(collectionName).Insert(tm);err!=nil{
		rnd.JSON(w,http.StatusProcessing, renderer.M{
			"message": "Failed saving the todo",
			"error": err,
		})
		return
	
	}

	// sending the todo response after creating successfully
	rnd.JSON(w, http.StatusCreated, renderer.M{
		"message": "Todo created successfully",
		"todo_id": tm.ID.Hex(),

	})


}

func deleteTodo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))

	if !bson.IsObjectIdHex(id) {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "The id is invalid",
		})
		return
	}

	if err := db.C(collectionName).RemoveId(bson.ObjectIdHex(id)); err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to delete todo",
			"error":   err,
		})
		return
	}

	rnd.JSON(w, http.StatusOK, renderer.M{
		"message": "Todo deleted successfully",
	})
}

func main(){
	stopChan := make(chan os.Signal, 1) // Add buffer size of 1
	signal.Notify(stopChan, os.Interrupt)      // handling the server stops gracefully

	r:= chi.NewRouter()
	r.Use(middleware.Logger)
	r.Get("/", homeHandler)
	r.Mount("/todo",todoHandlers())

	srv := &http.Server{
		Addr: port,
		Handler: r,
		ReadTimeout: 60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout: 120 * time.Second,
	}
	
	go func(){
		log.Println("Server is running on port",port)
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("Server error: %v\n",err)
		}
	}()

	<-stopChan
	log.Println("Server is shutting down....")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	srv.Shutdown(ctx)
	defer cancel()
		log.Println("Server is shutting down gracefully")
	
}

func updateTodo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))

	if !bson.IsObjectIdHex(id) {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "The id is invalid",
		})
		return
	}

	var t todo

	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		rnd.JSON(w, http.StatusProcessing, err)
		return
	}

	// simple validation
	if t.Title == "" {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "The title field is requried",
		})
		return
	}

	// if input is okay, update a todo
	if err := db.C(collectionName).
		Update(
			bson.M{"_id": bson.ObjectIdHex(id)},
			bson.M{"title": t.Title, "completed": t.Completed},
		); err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to update todo",
			"error":   err,
		})
		return
	}

	rnd.JSON(w, http.StatusOK, renderer.M{
		"message": "Todo updated successfully",
	})
}



func todoHandlers() http.Handler {
	rg:= chi.NewRouter()
	rg.Group(func(r chi.Router){
		r.Get("/",fetchTodos)
		r.Post("/",createTodo)
		r.Put("/{id}",updateTodo)
		r.Delete("/{id}",deleteTodo)
	})
	return rg
} 


func checkErr(err error){
	if err != nil {
		log.Fatal(err)
	}
}