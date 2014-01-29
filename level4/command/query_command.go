package command

import (
        "github.com/goraft/raft"
        "stripe-ctf.com/sqlcluster/sql"
        "fmt"

)


func init() {
        raft.RegisterCommand(&QueryCommand{})
}


// This command writes a value to a key.
type QueryCommand struct {
        Query   string `json:"query"`
}



// Creates a new write command.
func NewQueryCommand(query string) *QueryCommand {
        return &QueryCommand{
                Query:   query,
        }
}

// The name of the command in the log.
func (c *QueryCommand) CommandName() string {
        return "foobarbaz"
}

// Writes a value to a key.
func (c *QueryCommand) Apply(server raft.Server) (interface{}, error) {
        
        sql := server.Context().(*sql.SQL)
        output, _ := sql.Execute(c.Query)
        formatted := fmt.Sprintf("SequenceNumber: %d\n%s", output.SequenceNumber, output.Stdout)
        return []byte(formatted), nil
}
