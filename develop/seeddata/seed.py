from chance import chance
import json
import requests
from requests.auth import HTTPBasicAuth

def genUser():
  username = chance.string(pool="abcdefghijklmnopqrstuvwxyz", minimum=5, maximum=20)
  first_name = chance.first()
  last_name = chance.last()
  email = chance.email()
  password = chance.string(minimum=5, maximum=20)
  user = { "username": username, "first_name": first_name, "last_name": last_name, "email": email, "password": password }
  return user

def genRepo():
  name = chance.word(language="en") + chance.word(language="en")
  description = chance.sentence()
  version = chance.character(pool="vV") + chance.character(pool="012345") + "." + chance.character(pool="012345") + "." + chance.character(pool="012345")
  url = chance.url(dom="http://github.com", exts=["hcl", "nomad"])
  repo = {"name": name, "description": description, "version": version, "url": url}
  return repo

for u in range(100):
  user = genUser()
  # print("Initial login details")
  # print(user[1])
  # print(user[2])
  # posturl = "http://host.docker.internal:10000/auth/signup"
  posturl = "http://localhost:10000/auth/signup"
  response = requests.post(posturl, data=json.dumps(user), headers={"Content-Type": "application/json"})
  resp = response.json()
  print(resp)
  userdata = resp["Data"]
  # loginurl = "http://host.docker.internal:10000/auth/login"
  loginurl = "http://localhost:10000/auth/login"
  login_user = {  "identity": user["email"], "password": user["password"]}
  # print("Userdetails for POST Method")
  # print(login_user)
  login_response = requests.post(loginurl, data=json.dumps(login_user), headers={"Content-Type":"application/json"})
  l_response = login_response.json()
  print("Login Response")
  print(l_response)
  authtoken = l_response["Data"]["Token"]
  print("Authtoken")
  print(authtoken)
  for repo in range(10):
      repo = genRepo()
      username = userdata["username"]
      auth_headers = {"Content-Type": "application/json","Authorization" : "Bearer "+ authtoken}
      # url = "http://host.docker.internal:10000/"+username
      url = "http://localhost:10000/"+username
      response = requests.post(url, headers=auth_headers, json=repo)
      print(response.json())
