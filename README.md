
How to build
------------

### Using "go get"

    go get github.com/nmandery/honeybee/cmd/honeybee

Once installed, the compiled executable is located in $GOPATH/bin/honeybee. To use the program, either
copy it to the desired location, or ensure $GOPATH/bin is in your $PATH and run the executable using:

    honeybee


### Without using "go get"

    cd $GOPATH/src
    mkdir -p github.com/nmandery
    cd github.com/nmandery
    git clone https://github.com/nmandery/honeybee
    cd honeybee/cmd/honeybee
    go build
    # now there is a built executable in the current directory
