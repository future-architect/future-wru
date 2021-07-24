import csv
with open('testuser.csv', 'w', newline='') as file:
    writer = csv.writer(file)
    writer.writerow(["id", "name", "mail", "org", "scopes", "twitter", "github"])
    writer.writerow(["user1", "test user", "user1@example.com", "R&D", "admin,user,org:rd", "user1", "user1"])
