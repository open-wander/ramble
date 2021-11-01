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
  * add the JWT Secret passphrase
  * Enable logging if you need it.
  * Configure your Postgres DB details
* run ./rmbl-server

## Features

List of features ready and TODOs for future development

* Provides an API endpoint for querying the registry
* When adding a new repository it downloads the URL's and Readme

## API Endpoints
`/auth/signup`
POST = user signup

### Payload should look like this

```json
{
  "username": "user",
  "email": "email@email.com",
  "password": "password"
}
```

`/auth/login`
POST = user login

### Payload should look like this

```json
{
  "identity": "email@email.com",
  "password": "password"
}
```

`/`
GET = Gets list of all repos

`/:org`
GET = Gets the details of all Org repositories

`/:org/:reponame`
GET = Gets the details of a Specific repo

`/:org/`
POST = Create a new repo entry with the following payload

### Payload should look like this

```JSON
{
  "name": "fabio_lb",
  "user": "rmbl",
  "version": "0.2.1",
  "description": "Fabio LoadBalancer",
  "url": "https://github.com/rmbl/fabio_lb"
}
```

`/:org/:name`
PUT = Update a Repo to the latest details.

### Payload should look like this

```JSON
{
  "name": "fabio_lb",
  "user": "rmbl",
  "version": "0.2.1",
  "description": "Fabio LoadBalancer",
  "url": "https://github.com/rmbl/fabio_lb"
}
```

`/:org/:name`
DELETE = Delete a repo that is no longer needed. (only marks it as deleted at the moment)


`/?limit=25&offset=0&order=DESC&search=hello`
Search Repos using the search term.
You can stipulate the following:

* Limit=25 (default)
* Offset=0
* Sort=ID (Sort Field)
* Order=ASC/DESC

`/:org/?limit=25&offset=0ID&order=DESC&search=hello`
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
