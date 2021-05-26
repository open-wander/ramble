from chance import chance
import json
import requests

def genUser():
  username = chance.first()
  email = chance.email()
  password = chance.string(minimum=5, maximum=20)
  names = chance.name()
  user = {"username": username, "email": email, "password": password, "names": names}
  return user

def getUser(username):
  requests.get('http://host.docker.internal:10000/_catalog', params=params)

def genRepo():
  name = chance.word(language='en')
  description = chance.sentence()
  version = chance.character(pool='vV') + chance.character(pool='012345') + "." + chance.character(pool='012345') + "." + chance.character(pool='012345')
  url = chance.url(dom='github.com', exts=['hcl', 'nomad'])
  repo = {"name": name, "description": description, "version": version, "url": url}
  return repo

for u in range(10):
  user = genUser()
  print(user['username'])
  print(user['password'])
  posturl = 'http://host.docker.internal:10000/user/'
  response = requests.post(posturl, data=user)
  params = {"format": "json"}
  geturl = 'http://host.docker.internal:10000/user/'
  siteusers = requests.get(geturl, params)
  print("Get Site Users")
  userlist = siteusers.json()
  print(siteusers.json())
  for userid in range(len(userlist['data'])):
    userdata = userlist['data'][userid]
    # print(userdata['id'])
    # print(userdata["username"])
    usersid = userdata['id']
    for repo in range(10):
      repo = genRepo()
      # print(repo)
      username = userdata["username"]
      url = 'http://host.docker.internal:10000/'+username
      response = requests.post(url, json=repo)