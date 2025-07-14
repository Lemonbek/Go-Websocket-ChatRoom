var selectedChat = "general";

class Event{
  constructor(type,payload) {
    this.type = type;
    this.payload = payload;
  }
}

// Add helper to create message element
function sendVote(msgId, voteType) {
  sendEvent("vote_message", {
    id: msgId,
    chatroom: selectedChat,
    voteType: voteType
  });
}

// Update createMessageElement to use sendVote
function createMessageElement({username, message, timestamp, likes = 0, dislikes = 0, id = ''}) {
  const msgDiv = document.createElement('div');
  msgDiv.className = 'chat-message';
  if (id) msgDiv.dataset.msgid = id;

  const headerDiv = document.createElement('div');
  headerDiv.className = 'chat-message-header';

  const userSpan = document.createElement('span');
  userSpan.className = 'chat-username';
  userSpan.textContent = username || 'anon';

  const timeSpan = document.createElement('span');
  timeSpan.className = 'chat-timestamp';
  if (timestamp) {
    try {
      const d = new Date(timestamp);
      timeSpan.textContent = d.toLocaleTimeString([], {hour: '2-digit', minute:'2-digit', second:'2-digit'});
    } catch { timeSpan.textContent = '' }
  } else {
    timeSpan.textContent = '';
  }

  headerDiv.appendChild(userSpan);
  headerDiv.appendChild(timeSpan);

  const textDiv = document.createElement('div');
  textDiv.className = 'chat-text';
  textDiv.textContent = message;

  // Voting controls
  const voteDiv = document.createElement('div');
  voteDiv.className = 'chat-votes';

  const upBtn = document.createElement('button');
  upBtn.className = 'vote-btn upvote';
  upBtn.innerHTML = '▲';
  const likeCount = document.createElement('span');
  likeCount.className = 'vote-count like-count';
  likeCount.textContent = likes;

  const downBtn = document.createElement('button');
  downBtn.className = 'vote-btn downvote';
  downBtn.innerHTML = '▼';
  const dislikeCount = document.createElement('span');
  dislikeCount.className = 'vote-count dislike-count';
  dislikeCount.textContent = dislikes;

  // Send vote event to backend
  upBtn.onclick = function() {
    if (id) sendVote(id, "up");
  };
  downBtn.onclick = function() {
    if (id) sendVote(id, "down");
  };

  voteDiv.appendChild(upBtn);
  voteDiv.appendChild(likeCount);
  voteDiv.appendChild(downBtn);
  voteDiv.appendChild(dislikeCount);

  msgDiv.appendChild(headerDiv);
  msgDiv.appendChild(textDiv);
  msgDiv.appendChild(voteDiv);
  return msgDiv;
}

// Update routeEvent to update votes if message already exists
function routeEvent(event){
  if (event.type === undefined){
    alert('no type field in the event')
  }
  switch(event.type){
    case "new_message":
      const chatBox = document.getElementById("chatmessages");
      let payload = event.payload;
      if (typeof payload === 'string') {
        try { payload = JSON.parse(payload); } catch {}
      }
      let messageText = payload.message || event.payload.message || event.payload;
      let username = payload.username || 'anon';
      let timestamp = payload.timestamp || '';
      let likes = payload.likes || 0;
      let dislikes = payload.dislikes || 0;
      let id = payload.id || '';
      // If message with this id exists, update its votes
      if (id) {
        const existing = chatBox.querySelector(`[data-msgid='${id}']`);
        if (existing) {
          const likeCount = existing.querySelector('.like-count');
          const dislikeCount = existing.querySelector('.dislike-count');
          if (likeCount) likeCount.textContent = likes;
          if (dislikeCount) dislikeCount.textContent = dislikes;
          return;
        }
      }
      // Otherwise, add new message
      const msgElem = createMessageElement({username, message: messageText, timestamp, likes, dislikes, id});
      chatBox.appendChild(msgElem);
      chatBox.scrollTop = chatBox.scrollHeight;
      break;
    default:
      alert("unsupported message type");
      break;
  }
}
function sendEvent(eventName, payload) {
  const event = new Event(eventName, payload);
  console.log("Sending event:", event); // Add this line
  conn.send(JSON.stringify(event));
}
// In changeChatRoom, clear the chat thread div
function changeChatRoom() {
  var newChat = document.getElementById("chatroom");
  if (newChat != null && newChat.value != selectedChat) {
    selectedChat = newChat.value;
    document.getElementById("chat-header").innerText = `Currently in chat: ${selectedChat}`;
    // Clear chat area
    document.getElementById("chatmessages").innerHTML = "";
    // Request chat history for the new chatroom
    sendEvent("join_chatroom", { chatroom: selectedChat });
  }
  return false;
}

function sendMessage() {
  var newMessage = document.getElementById("message");
  if (newMessage != null) {
    sendEvent("send_message", { username: currentUsername, message: newMessage.value, chatroom: selectedChat });
  }
  return false;
}

function login(){
let formData = {
  "username": document.getElementById("username").value,
  "password": document.getElementById("password").value
}
fetch("login",{
  method: 'post',
  body: JSON.stringify(formData),
  mode:'cors'
}).then((response) => {
  if(response.ok){
    return response.json();
  }else {
    throw "unauthorized";
  }
  }).then((data)=>{
    //we are authenticated
    currentUsername = formData.username;
    connectWebsocket(data.otp)
  }).catch((e) => {alert(e)});
return false


}
function connectWebsocket(otp){

    if(window["WebSocket"]){
      //connect ws
      console.log("websocket Supported")
      conn = new WebSocket("ws://" +document.location.host + "/ws?otp=" +otp)
      conn.onopen = function (evt){
        document.getElementById("connection-header").innerHTML = "Connected to websocket = true"
        // Request chat history for the default chatroom on connect
        sendEvent("join_chatroom", { chatroom: selectedChat });
      }
      conn.onclose = function (evt){
        document.getElementById("connection-header").innerHTML = "Connected to websocket = false"
        //reconnect
      }
      conn.onmessage= function(evt){
        const eventData = JSON.parse(evt.data);
        const event = Object.assign(new Event, eventData)
        routeEvent(event)
      }
    }else{
      console.log("Browser doesn't support websocket")
    }

  }
window.onload = function() {
  document.getElementById("chatroom-selection").onsubmit = changeChatRoom
  document.getElementById("chatroom-message").onsubmit = sendMessage
  document.getElementById("login-form").onsubmit = login
}
