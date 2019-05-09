JSONFILE=$(ls | grep .json)
export GOOGLE_APPLICATION_CREDENTIALS="$GOPATH/src/github.com/matheusbortolini/babel-io/$JSONFILE"
