# Nomad Specification Registry

> This project is an attempt to create a Nomad Job Specification Registry!

## Table of contents

* [General info](#general-info)
* [Screenshots](#screenshots)
* [Technologies](#technologies)
* [Setup](#setup)
* [Features](#features)
* [Status](#status)
* [Inspiration](#inspiration)
* [Contact](#contact)

## General info

I started this project as I wanted to have a central space where people who were looking to work with Nomad could easily find job specifications that are known to be deployed in a production ready way.

## Screenshots

![Example screenshot](./img/screenshot.png)

## Technologies

* Tech 1 - Golang

## Setup

* Download the release from the release site
* Make a copy of the config.yml.example file and rename it to config.yml
* Edit the file and:
  * add the Server port
  * add the Github username that owns the repositories
  * add the Github auth token for acessing the github API
* run ./rmbl-server

## Features

List of features ready and TODOs for future development

* Provides an API endpoint for querying the registry
* When adding a new repository it downloads the URL's and Readme

## API Endpoints

`/v1/_catalog`
GET = Gets list of all repos

`/v1/:user`
GET = Gets the details of all Org repositories

`/v1/:user/:name`
GET = Gets the details of a Specific repo

`/v1/:user/`
POST = Create a new repo entry with the following payload

```JSON
{
  "name": "fabio_lb",
  "user": "nsreg",
  "version": "0.2.1",
  "description": "Fabio LoadBalancer",
  "url": "https://github.com/nsreg/fabio_lb"
}
```

`/v1/:user/:name`
PUT = Update a Repo to the latest details.

### Payload should look like this

```JSON
{
  "name": "fabio_lb",
  "user": "nsreg",
  "version": "0.2.1",
  "description": "Fabio LoadBalancer",
  "url": "https://github.com/nsreg/fabio_lb"
}
```

`/v1/:user/:name`
DELETE = Delete a repo that is no longer needed. (only marks it as deleted at the moment)


## - - - TODO - - - -

`/v1/?limit=25&offset=0&sort=ID&order=DESC&search=hello`
Search Repos using the search term.
You can stipulate the following:

* Limit=25 (default)
* Offset=0
* Sort=ID (Sort Field)
* Order=ASC/DESC

`/v1/:user/?limit=25&offset=0&sort=ID&order=DESC&search=hello`
Search Repos using the search term.
You can stipulate the following:

* Limit=25 (default)
* Offset=0
* Sort=ID (Sort Field)
* Order=ASC/DESC

## Status

Project is: _in progress_

## Inspiration

The HashiCorp Nomad Team

## Contact

Created by [@lhaig](https://haigmail.com/) - feel free to contact me!
