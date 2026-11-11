try {
  rs.status();
} catch (error) {
  if (error.code !== 94 && error.codeName !== "NotYetInitialized") throw error;
  rs.initiate({_id: "rs0", members: [{_id: 0, host: "mongo:27017"}]});
}
let ready = false;
for (let i = 0; i < 60; i++) {
  if (db.hello().isWritablePrimary) { ready = true; break; }
  sleep(1000);
}
if (!ready) throw new Error("MongoDB primary did not become ready");
print("Replica set rs0 ready");
