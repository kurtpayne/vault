package database

import (
	"fmt"
	"strings"

	"github.com/hashicorp/vault/helper/strutil"
	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

func pathListRoles(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "roles/?$",

		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.ListOperation: b.pathRoleList,
		},

		HelpSynopsis:    pathRoleHelpSyn,
		HelpDescription: pathRoleHelpDesc,
	}
}

func pathRoles(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "roles/" + framework.GenericNameRegex("name"),
		Fields: map[string]*framework.FieldSchema{
			"name": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "Name of the role.",
			},

			"sql": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "SQL string to create a user. See help for more info.",
			},
			
			"db_name": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "Name of the database associated with the role.",
			},
		},

		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.ReadOperation:   b.pathRoleRead,
			logical.UpdateOperation: b.pathRoleCreate,
			logical.DeleteOperation: b.pathRoleDelete,
		},

		HelpSynopsis:    pathRoleHelpSyn,
		HelpDescription: pathRoleHelpDesc,
	}
}

func (b *backend) Role(s logical.Storage, n string) (*roleEntry, error) {
	entry, err := s.Get("role/" + n)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	var result roleEntry
	if err := entry.DecodeJSON(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (b *backend) pathRoleDelete(
	req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	err := req.Storage.Delete("role/" + data.Get("name").(string))
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (b *backend) pathRoleRead(
	req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	role, err := b.Role(req.Storage, data.Get("name").(string))
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, nil
	}

	return &logical.Response{
		Data: map[string]interface{}{
			"sql": role.SQL,
		},
	}, nil
}

func (b *backend) pathRoleList(
	req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	entries, err := req.Storage.List("role/")
	if err != nil {
		return nil, err
	}

	return logical.ListResponse(entries), nil
}

func (b *backend) pathRoleCreate(
	req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	name := data.Get("name").(string)
	sqlstmt := data.Get("sql").(string)
	dbname := data.Get("db_name").(string)
	
	// Get our connection
	dbconn, err := b.DBConnection(req.Storage, dbname)
	if dbconn == nil {
		b.logger.Trace("[TRACE] b.dbs[%s] is not connected.", dbname)
		return nil, err
	}

	// Test the query by trying to prepare it
	for _, query := range strutil.ParseArbitraryStringSlice(sqlstmt, ";") {
		query = strings.TrimSpace(query)
		if len(query) == 0 {
			continue
		}
		
		stmt, err := dbconn.Prepare(Query(query, map[string]string{
			"name":       "foo",
			"password":   "bar",
			"expiration": "",
		}))
		if err != nil {
			return logical.ErrorResponse(fmt.Sprintf(
				"Error testing query: %s", err)), nil
		}
		stmt.Close()
	}

	// Store it
	entry, err := logical.StorageEntryJSON("role/"+name, &roleEntry{
		SQL:    sqlstmt,
		DBName: dbname,
	})
	if err != nil {
		return nil, err
	}
	if err := req.Storage.Put(entry); err != nil {
		return nil, err
	}

	return nil, nil
}

type roleEntry struct {
	// SQL statement for the role
	SQL    string `json:"sql"`
	
	// Name of database that will use the role
	DBName string `json:"db_name"`
}

const pathRoleHelpSyn = `
Manage the roles that can be created with this backend.
`

const pathRoleHelpDesc = `
This path lets you manage the roles that can be created with this backend.

The "sql" parameter customizes the SQL string used to create the role.
This can be a sequence of SQL queries. Some substitution will be done to the
SQL string for certain keys. The names of the variables must be surrounded
by "{{" and "}}" to be replaced.

  * "name" - The random username generated for the DB user.

  * "password" - The random password generated for the DB user.

  * "expiration" - The timestamp when this user will expire.

Example of a decent SQL query to use:

	CREATE ROLE "{{name}}" WITH
	  LOGIN
	  PASSWORD '{{password}}'
	  VALID UNTIL '{{expiration}}';
	GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO "{{name}}";

Note the above user would be able to access everything in schema public.
For more complex GRANT clauses, see the PostgreSQL manual.
`