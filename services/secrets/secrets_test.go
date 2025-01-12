package secrets_test

import (
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type SecretsTestSuite struct {
	test_utils.TestSuite
}

func (self *SecretsTestSuite) TestSecretsService() {
	secrets, err := services.GetSecretsService(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Define a secret - invalid verifier
	err = secrets.DefineSecret(self.Ctx, "MySecretType", "invalid VQL")
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), `Invalid verifier lambda:`)

	// Empty verifier is ok
	err = secrets.DefineSecret(self.Ctx, "MySecretType", "")
	assert.NoError(self.T(), err)

	// Add a verifier that requires a field matched a certain format
	err = secrets.DefineSecret(self.Ctx,
		"MySecretType", "x=>x.MyField =~ 'FieldFormat'")
	assert.NoError(self.T(), err)

	// Now add an invalid secret.
	scope := vql_subsystem.MakeScope()
	err = secrets.AddSecret(self.Ctx, scope,
		"MySecretType", "MySecret", ordereddict.NewDict().
			Set("InvalidField", "Some value"))
	assert.Error(self.T(), err)

	err = secrets.AddSecret(self.Ctx, scope,
		"MySecretType", "MySecret", ordereddict.NewDict().
			Set("MyField", "Valid FieldFormat"))
	assert.NoError(self.T(), err)

	golden := ordereddict.NewDict()
	db := test_utils.GetMemoryDataStore(self.T(), self.ConfigObj)

	golden.Set("Added Secret", getData(self.ConfigObj, db,
		"config/secrets/MySecretType/MySecret"))

	// Grant the secret to two users
	err = secrets.ModifySecret(self.Ctx,
		&api_proto.ModifySecretRequest{
			TypeName: "MySecretType",
			Name:     "MySecret",
			AddUsers: []string{"User1", "User2"}})
	assert.NoError(self.T(), err)

	golden.Set("Granted Secret", getData(self.ConfigObj, db,
		"config/secrets/MySecretType/MySecret"))

	// User2 asks for the secret
	secret_data, err := secrets.GetSecret(
		self.Ctx, "User2", "MySecretType", "MySecret")
	assert.NoError(self.T(), err)

	golden.Set("SecretData", secret_data)

	// Revoke user2
	err = secrets.ModifySecret(self.Ctx,
		&api_proto.ModifySecretRequest{
			TypeName:    "MySecretType",
			Name:        "MySecret",
			RemoveUsers: []string{"User2"}})
	assert.NoError(self.T(), err)

	golden.Set("Revoked Secret", getData(self.ConfigObj, db,
		"config/secrets/MySecretType/MySecret"))

	// User2 asks for the secret again - this time denied
	secret_data, err = secrets.GetSecret(
		self.Ctx, "User2", "MySecretType", "MySecret")
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), `Permission Denied`)

	goldie.Assert(self.T(), "TestSecretsService",
		json.MustMarshalIndent(golden))
}

func getData(
	config_obj *config_proto.Config,
	db *datastore.MemcacheDatastore,
	path string) *ordereddict.Dict {

	path_spec := path_specs.NewUnsafeDatastorePath(strings.Split(path, "/")...)
	b, _ := db.GetBuffer(config_obj, path_spec)

	result := ordereddict.NewDict()
	json.Unmarshal(b, result)

	return result
}

func TestSecretsService(t *testing.T) {
	suite.Run(t, &SecretsTestSuite{})
}
