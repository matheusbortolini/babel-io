JSONFILE=$(ls | grep .json)
export GOOGLE_APPLICATION_CREDENTIALS="$GOPATH/src/github.com/matheusbortolini/babel-io/$JSONFILE"

CMD="dep version"
if $CMD | grep -q 'platform'; then
     echo "dep already installed"
 else
  set -x
   $(go get -u github.com/golang/dep)
fi
