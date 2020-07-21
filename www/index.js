// Converts bytes to a human readable value.
function bytesToHuman(bytes) {
    var i = Math.floor(Math.log(bytes) / Math.log(1024)),
    sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];

    return (bytes / Math.pow(1024, i)).toFixed(2) * 1 + ' ' + sizes[i];
}

// A timer for when messages are shown on screen to auto hide.
var messageTimer = null;

// Hide the message on screen.
function hideMessage() {
    // Get the message div.
    var message = document.getElementById("message");
    message.style.display = "none"; // Do not display.
    // Clear the message timer.
    clearTimeout(messageTimer);
    messageTimer = null;
}

// Display a standard message on screen.
function displayMessage(message) {
    displayMessage(message, "cadetblue", true);
}

// Display a sucessful message with green color.
function displaySuccess(message) {
    displayMessage(message, "green", true);
}

// Display an error message with red color.
function displayError(message) {
    displayMessage(message, "red", true);
}

// Display a message with a timeout and color specified.
function displayMessage(message, color, timeout) {
    // If no color defined, we use cadetblue.
    if (color == undefined) {
        color = "cadetblue";
    }
    // Log the message to the javascript console.
    console.log(message);

    // Get the message div.
    var messageDiv = document.getElementById("message");
    messageDiv.innerText = message;
    messageDiv.style.backgroundColor = color;
    messageDiv.style.display = "block"; // Make message visable.

    // If a message timer already exists, we can clear the timeout to prevent it from hiding this message.
    if (messageTimer!=null) {
        clearTimeout(messageTimer);
    }
    // If message is to timeout, set a timeout to hide in 5 seconds.
    if (timeout) {
        messageTimer = setTimeout(hideMessage, 5000);
    }
}

// Configuration Options
var UIDisableSpamReporting = false;
var UIDisableLogs = false;

// The width calculated for the subject.
var UISubjectWidth = 0;

// Build custom CSS based on configuration.
function rebuildCustomCSS() {
    var cssConfig = '<style type="text/css">';
    if (UIDisableSpamReporting) {
        cssConfig += `
        #mailLearnHamButton {
            display: none;
        }
        #mailLearnSpamButton {
            display: none;
        }
        #mailDownloadButton {
            border-top-right-radius: 0.25rem;
            border-bottom-right-radius: 0.25rem;
        }
        `;
    }
    if (UIDisableLogs) {
        cssConfig += `
        .nav-link.log {
            display: none;
        }
        #message_list th.status, #message_list td.status {
            display: none;
        }
        `;
    }
    if (UISubjectWidth!=0) {
        cssConfig += `
        #message_list th.subject, #message_list td.subject {
            width: ${UISubjectWidth}px;
            max-width: ${UISubjectWidth}px;
        }
        `;
    }
    cssConfig += "</style>";
    // Add style tag to page.
    document.getElementById("customcss").innerHTML = cssConfig;
}

// Load configuration from API.
function loadConfig() {
    // Call the API.
    $.ajax({
        dataType: "json",
        type: "GET",
        url: "/api/config"
    })
    .done(function(data) {
        // If an error ocurred. Display it.
        if (data.status=="error") {
            displayError("Unable to load configuration: "+data.error);
            return;
        }

        // Save configuration.
        UIDisableSpamReporting = data.disable_spam_reporting;
        UIDisableLogs = data.disable_logs;

        if (data.custom_brand!="") {
            $("#navbar_brand").text(data.custom_brand);
        }
        $("#message_count").text(data.message_count.toLocaleString());

        // Rebuild CSS with new config.
        rebuildCustomCSS();
    })
    .fail(function(jqXHR, textStatus) {
        // On error, display a message.
        displayError("Unable to load configuration: "+textStatus);
    });
}

// Storage of the currently selected email message.
var selectedMessage = null;

// Auto resize global resize based variable.
// This variable is basically the current top offset of the message contents view, minus the message list height.
// This allows us to easily determine the max height of the message contents view by taking the window height
//  and substracting the message height and this base height.
var messageResizeBase = 0;

// Handle a window resize event.
function handleResize() {
    // Get the current message list height from either the message list container itself, or storage.
    var messagesH = $("#message_list_container").height();
    if (localStorage && 'message_list_height' in localStorage) {
        messagesH = localStorage.message_list_height;
        $("#message_list_container").height(messagesH);
    }
    // If we don't have a resize base already calculated, calculate it.
    if (messageResizeBase==0) {
        messageResizeBase = $("#message_contents").offset().top-messagesH;
    }
    // The new message contents height should be the window height minus messages list height minus the resize base.
    var messageH = $(window).height()-messagesH-messageResizeBase;
    $("#message_contents").height(messageH);

    // Get width of other columns.
    var fromWidth = $("th.from").width();
    var toWidth = $("th.to").width();
    var statusWidth = 0;
    if ($("th.status").is(":visible")) {
        statusWidth = $("th.status").width();
    }
    var receivedWidth = $("th.received").width();
    // Subtract width of other volumes from windows width.
    UISubjectWidth = $(window).width()-(fromWidth+toWidth+statusWidth+receivedWidth+16); // 16 is padding.
    // Limit to width of 100 pixels.
    if (UISubjectWidth<100) {
        UISubjectWidth = 100;
    }
    // Build the css.
    rebuildCustomCSS();
}

// This function will check to see where the active message is in the list, and determine if it is visable.
// If the message is not visable in the message list, it will scroll to make it visable.
function scollToActiveMessageIfNeeded() {
    // Get the current selection, and stop if no selection is made.
    var selection = $("#message_list .active");
    if (selection.length<=0) {
        return;
    }

    // Determine the selection's position in the container list.
    var selectionTop =  selection.position().top;
    var rowH = selection.height();
    var container = $("#message_list_container");

    // If the message is above the scroll position, we need to scroll up.
    if (container.scrollTop()>selectionTop-rowH) {
        container.animate({
            scrollTop: selectionTop-rowH
        }, 200);
    } else if (container.scrollTop()+container.height()<selectionTop+rowH) {
        // If the message is below the scroll position, we scroll down.
        container.animate({
            scrollTop: (selectionTop+rowH)-container.height()
        }, 200);
    }
}

// Global keyboard shortcut handler.
function handleKeydownEvent(e) {
    // Variable used to store what should be selected next.
    var nextSelection = null;

    // Handle keyboard events.
    if (e.which==40) {// If key down arrow.
        // Check if we have an active message.
        var active = $("#message_list .active");
        if (active.length==0) { // No active message, select first message in list.
            nextSelection = $("#message_list tbody tr").first()
        } else { // Active message, get the next entry.
            nextSelection = active.next()
        }
    } else if (e.which==38) { // If key up arrow.
        // Check if we have an active message.
        var active = $("#message_list .active");
        if (active.length==0) { // No active message, select last message in list.
            nextSelection = $("#message_list tbody tr").last();
        } else { // Active message, get the previous entry.
            nextSelection = active.prev();
        }
    }

    // If we have a next selection item, select it.
    if (nextSelection!=null && nextSelection.length!=0) {
        nextSelection.click();
        // Scroll to new selection if needed.
        scollToActiveMessageIfNeeded();
        // Stop propagating the keyboard event to additional dom objects.
        e.preventDefault();
    }
}

// This function activates the resizer element as a click/drag type element and adjusts all view sizes accordingly.
function makeMessageListResizable() {
    // If we do not currently have a height value stored in the local storage, let's add it.
    if (localStorage && !'message_list_height' in localStorage) {
        (localStorage.message_list_height = $("#message_list_container").height());
    }

    // This variable is used to determine if the mouse movements should actually resize the views.
    var isResizing = false;
    // Where the mousedown event was fired.
    var startingPosition = 0;
    // What we started with before resizing.
    var previousHeight = 0;

    // Register for the mousedown and mouseup events in the resizer element.
    $("#message_list_resizer")
    .mousedown(function(e) {
        // On mouse down, we store the starting position and current message list view height.
        isResizing = true;
        startingPosition = e.pageY;
        previousHeight = $("#message_list_container").height();
    })
    .mouseup(function(e) {
        // Now that we are done resizing, we can store the new height.
        isResizing = false;

        // Calculate new height.
        var newHeight = previousHeight+(e.pageY-startingPosition);
        // Set new height.
        $("#message_list_container").height(newHeight);
        // Store new height in local storage.
        localStorage && (localStorage.message_list_height = newHeight);

        // Adjust the message contents height according to math mentioned in handleResize().
        var messageH = $(window).height()-newHeight-messageResizeBase;
        $("#message_contents").height(messageH);
    });
    // Register for the document mouse move event as we will miss some mouse move events if we did so with the resizer element.
    $(document).mousemove(function(e) {
        // If we're not resizing, we should break here.
        if (!isResizing) {
            return
        }
        // Calculate new height of message list.
        var newHeight = previousHeight+(e.pageY-startingPosition);
        $("#message_list_container").height(newHeight);

        // Caculate new height of message contents.
        var messageH = $(window).height()-newHeight-messageResizeBase;
        $("#message_contents").height(messageH);
    });
}

// This variable is set to true if there are new messages to be loaded or if the 1 minute timer is triggered.
// We check this variable every 5 seconds to avoid refreshing too often on heavy email intake.
var shouldRefresh = false;
// During search input, it is possible that text is typed before a response
//  from the server from the last query was issued.
// This variable allows us to tell the load messages function to load again after
//  it has finnished loading this request.
var shouldSearch = false;
// Keep track as to rather the message list is already loading to prevent concurrent requests.
var loading = false;

// This function loads the list of messages from the API and renders them.
function loadMessageList() {
    // If we are already loading, we cannot do concurrent loads.
    if (loading) {
        return;
    }

    // Set the fact that we are loading to prevent additional loads.
    loading = true;

    // Reset all variables as we are loading now.
    shouldRefresh = false;
    shouldSearch = false;

    // Get the search query from the search input.
    var query = $("#searchInput").val();
    var data = {};
    if (query!="") {
        data["q"] = query;
    }

    // Send the request.
    $.ajax({
        dataType: "json",
        type: "GET",
        url: "/api/message_log",
        data: data
    })
    .done(function(data) {
        // If an error while loading, we can display the error.
        if (data.status=="error") {
            displayError("Unable to pull messages: "+data.error);
            // We are no longer loading.
            loading = false;
            // If search query was updated during loading, we need to re-load the message list.
            if (shouldSearch) {
                loadMessageList();
            }
            return;
        }

        // Get the message template.
        var template = $("#message_list_message_template").html();
        // Get the message list table body.
        var messageList = $("#message_list tbody");
        // Empty the body for new contents.
        messageList.html("");

        // Read the messages.
        for (var i=0; i<data.messages.length; i++) {
            var message = data.messages[i];
            // Format the received time to a readable format.
            message.formatted_date = moment(message.received).format('YYYY-MM-DD HH:mm:ss');
            // Add the message encoded with JSON for use on message selection.
            message.encoded_message = JSON.stringify(message);
            // Render the message and append it to the body.
            messageList.append(Mustache.render(template, message));
        }

        // If a message was selected, try and activate it in the message list if visable.
        if (selectedMessage!=null) {
            $("#"+selectedMessage.uuid).addClass("active");
            // Scroll to selected message if needed.
            scollToActiveMessageIfNeeded();
        }
        // We are no longer loading.
        loading = false;
        // If search query was updated during loading, we need to re-load the message list.
        if (shouldSearch) {
            loadMessageList();
        }
    })
    .fail(function(jqXHR, textStatus) {
        // On failure, we need to display a message.
        displayError("Unable to pull messages: "+textStatus);
        // We are no longer loading.
        loading = false;
        // If search query was updated during loading, we need to re-load the message list.
        if (shouldSearch) {
            loadMessageList();
        }
    });
}

// When the search box has received input, we need to re-load the messages.
function handleSearchInput() {
    // If we are already loading the message list, we need to load again after it completes.
    if (loading) {
        shouldSearch = true;
    } else {
        // Load messages now.
        loadMessageList();
    }
}

// Every 5 seconds, we check to see if we need to refresh the message list.
function checkIfRefreshNeeded() {
    // If we need to refresh, then we load the message list again.
    if (shouldRefresh) {
        loadMessageList();
    }
}

// This function handles the connection to the websocket and reconnects if needed.
function connectToWS() {
    // Connect to the websockets address.
    displayMessage("Connecting to websockets daemon");
    var ws = new WebSocket("ws://"+document.location.host+"/ws");

    // On error, we can display the error and close the connection.
    ws.onerror = function(err) {
        displayError(err, "red", false);
        ws.close();
    }

    // On connection complete, we just display a message.
    ws.onopen = function() {
        displaySuccess("Connected to websockets daemon");
    }

    // When the connection is closed, we need to try reconnecting in 5 seconds.
    ws.onclose = function() {
        displayMessage("Websockets connection closed", "cadetblue", false);
        setTimeout(connectToWS, 5000);
    }

    // When we receive a message, we need to parse it.
    ws.onmessage = function(event) {
        // Parse the json data.
        var message = JSON.parse(event.data);
        // If parse error, we display a message.
        if (message==undefined) {
            displayError("Received weird response: "+event.data);
        } else {
            // On good message, we process it.
            switch (message.type) {
                case "messageStatusesUpdated": // A message's delievery status was updated.
                // Set that we need to refresh the message list.
                shouldRefresh = true;
                break;
                case "receivedNewMessage": // A new message has been received.
                // Set that we need to refresh the message list.
                shouldRefresh = true;
                break;
                case "updateMessageCount": // The message count has changed.
                // Update the emssage count header.
                $("#message_count").text(message.msg.toLocaleString());
                default:
                // If we do not have a condition for the message, we just log it to the javascript console.
                console.log(message);
            }
        }
    }
}

// Load a message by its UUID.
function loadMessage(UUID) {
    // If the message being loaded is already selected, we can stop here.
    if (selectedMessage!=null && selectedMessage.uuid==UUID) {
        return;
    }

    // Call the API for the message.
    $.ajax({
        dataType: "json",
        type: "GET",
        url: "/api/message/"+UUID
    })
    .done(function(data) {
        // If an error ocurred. Display it.
        if (data.status=="error") {
            displayError("Unable to pull message: "+data.error);
            return;
        }
        // Update the selected message to this one.
        selectedMessage = data.message

        if (selectedMessage!=null) {
            // Add a formated date for this message.
            selectedMessage.formatted_date = moment(selectedMessage.received).format('YYYY-MM-DD HH:mm:ss');

            // Update the selected message data.
            updateSelectedMessage();

            // Update the selected message in the message list.
            $("#message_list tr.active").removeClass("active");
            $("#"+selectedMessage.uuid).addClass("active");
            scollToActiveMessageIfNeeded();
        }
    })
    .fail(function(jqXHR, textStatus) {
        // On error, display a message.
        displayError("Unable to pull message: "+textStatus);
    });
}

// When a source tab is selected, we need to grab the source from the API.
function handleSourceSelection() {
    // If no message is selected, we should stop here.
    if (selectedMessage==null) {
        return;
    }

    // Get the selected source type.
    var selection = $(this);
    // Update the active source type tab.
    $("#message_header .nav-tabs .active").removeClass("active");
    selection.addClass("active");

    // Determine the extension for selected soruce type.
    var extension = ".txt";
    if (selection.hasClass("html")) {
        extension = ".html";
    } else if (selection.hasClass("source")) {
        extension = ".eml";
    } else if (selection.hasClass("log")) {
        extension = ".log";
    }

    // If source type is HTML, we must do something special.
    if (extension==".html") {
        // Create an ifram with the html from the API.
        var iframe = $("<iframe>");
        iframe.css("height", "100%");
        iframe.css("width", "100%");
        iframe.attr("src",  "/api/message/"+selectedMessage.uuid+extension);

        // Append iframe to the message contents view.
        $("#message_contents").html("");
        $("#message_contents").append(iframe);
    } else {
        // All other source types are handled here.

        // Get the message source from the API.
        $.ajax({
            url: "/api/message/"+selectedMessage.uuid+extension
        })
        .done(function(data) {
            // If an error was returned, we display it.
            if (data.status!=undefined) {
                displayError("Unable to pull message: "+data.error);
                return;
            }

            // We display plain text message contents in a pre-formatted element.
            var preFormated = $("<pre>").text(data);

            // Append the pre-formatted element to the message contents.
            $("#message_contents").html("");
            $("#message_contents").append(preFormated);
        })
        .fail(function(jqXHR, textStatus) {
            // Om error, display a message.
            displayError("Unable to pull message: "+textStatus);
        });
    }
}

// This function is used to update the currently selected message view.
function updateSelectedMessage() {
    // Update the header information.
    $("#message_header .received").text(selectedMessage.formatted_date);
    $("#message_header .size").text(bytesToHuman(selectedMessage.size));
    $("#message_header .from").text(selectedMessage.from);
    $("#message_header .to").text(selectedMessage.to);
    $("#message_header .subject").text(selectedMessage.subject);
    $("#message_header .spam_score").text(selectedMessage.spam_score);
    $("#message_header .status").text(selectedMessage.status);
    $("#message_header .source_ip").text(selectedMessage.source_ip);

    // If no plain text, this must be a html email.
    if (!selectedMessage.plain_text) {
        // Disable plain text source selection tab.
        $("#message_header .nav-tabs .plaintext").prop("disabled", true);
        // Enable the html source selection tab.
        $("#message_header .nav-tabs .html").prop("disabled", false);
        // If the currently selected source tab is disabled, we need to select html.
        if ($("#message_header .nav-tabs .active").prop("disabled")) {
            $("#message_header .nav-tabs .html").click();
        } else {
            // Otherwise, select the active tab.
            $("#message_header .nav-tabs .active").click();
        }
    } else {
        // When plaintext is avaiable, we need to disable HTML source selection only if there is no HTML.
        $("#message_header .nav-tabs .html").prop("disabled", !selectedMessage.html);
        // We can enable the plain text source selection.
        $("#message_header .nav-tabs .plaintext").prop("disabled", false);
        // If the currently selected source tab is disabled, we need to select plain text.
        if ($("#message_header .nav-tabs .active").prop("disabled")) {
            $("#message_header .nav-tabs .plaintext").click();
        } else {
            // Otherwise, select the active tab.
            $("#message_header .nav-tabs .active").click();
        }
    }
}

// When a message is selected in the message list, this function is called.
function handleMessageListSelection() {
    // Get the selected message data.
    var selection = $(this);
    var message = JSON.parse(selection.attr("data"));
    selectedMessage = message;

    // Change the selelected message in the message list to this selection.
    $("#message_list tr.active").removeClass("active");
    selection.addClass("active");

    // Update the location hash URI to this message.
    window.location.hash = "uuid="+message.uuid;

    // Update the selected message.
    updateSelectedMessage();
}

// When the learn ham spam reporting button is clicked, this function is called.
function learnHam() {
    // If no message is selected, we display a message and stop here.
    if (selectedMessage==null) {
        displayMessage("Select a message first.");
        return;
    }
    // Confirm that this action is actually wanted to occur.
    var r = confirm("Are you sure you want to report as ham?");
    if (r != true) {
        return
    }

    // Send request to the API.
    $.ajax({
        dataType: "json",
        type: 'PUT',
        url: "/api/message/"+selectedMessage.uuid+"/learn_ham"
    })
    .done(function(data) {
        // If error, display message.
        if (data.status=="error") {
            displayError("Unable to report ham: "+data.error);
            return;
        }
        // We Successfully submitted a report.
        displaySuccess("Successfully reported as ham.");
    })
    .fail(function(jqXHR, textStatus) {
        // On error, display message.
        displayError("Unable to report ham: "+textStatus);
    });
}

// When the learn spam spam reporting button is clicked, this function is called.
function learnSpam() {
    // If no message is selected, we display a message and stop here.
    if (selectedMessage==null) {
        displayMessage("Select a message first.");
        return;
    }
    // Confirm that this action is actually wanted to occur.
    var r = confirm("Are you sure you want to report as spam?");
    if (r != true) {
        return
    }

    // Send request to the API.
    $.ajax({
        dataType: "json",
        type: "PUT",
        url: "/api/message/"+selectedMessage.uuid+"/learn_spam"
    })
    .done(function(data) {
        // If error, display message.
        if (data.status=="error") {
            displayError("Unable to report spam: "+data.error);
            return;
        }
        // We Successfully submitted a report.
        displaySuccess("Successfully reported as spam.");
    })
    .fail(function(jqXHR, textStatus) {
        // On error, display message.
        displayError("Unable to report spam: "+textStatus);
    });
}

// When the document has fully loaded, we get everything started.
$(document).ready(function() {
    // Connect to websockets if available.
    if (!window["WebSocket"]) {
        displayError("Your browser does not support websockets, auto refresh will only occur once every minute.");
        return;
    } else {
        connectToWS();
    }
    // Laod the configuration from API.
    loadConfig();

    // Make the message list resizer element work.
    makeMessageListResizable();

    // On window resize events, adjust view sizes.
    $(window).resize(handleResize);
    // Update the view sizes.
    setTimeout(handleResize, 200);

    // Load the message list.
    loadMessageList();

    // Every 5 seconds, we need to check if we need to refresh the message list.
    setInterval(checkIfRefreshNeeded, 5000);
    // Every minute, we force a refresh.
    setInterval(function() {
        shouldRefresh = true;
    }, 60000);

    // On input in the search field, handle it.
    $("#searchInput").on("input", handleSearchInput);

    // Handle clicking on items in the message list.
    $("#message_list").on("click", "tr", handleMessageListSelection);

    // Handle global document key down events.
    $(document).keydown(handleKeydownEvent);

    // Handle clicks on source selection tabs.
    $("#message_header .nav-tabs .nav-link").click(handleSourceSelection);

    // Handle a click on the email download button.
    $("#mailDownloadButton").click(function() {
        // If no message selected, stop here.
        if (selectedMessage==null) {
            return;
        }
        // Setup the download path.
        var downloadPath = "/api/message/"+selectedMessage.uuid+".eml";

        // Create an link with a download file name to allow downloading without navigating away.
        // This is done to avoid disconnection from the websocket.
        var a = document.createElement("a");
        a.href = downloadPath;
        a.download = downloadPath.substr(downloadPath.lastIndexOf('/') + 1);

        // Append the link, click it and remove it.
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
    });

    // Setup handlers for spam reporting.
    $("#mailLearnHamButton").click(learnHam);
    $("#mailLearnSpamButton").click(learnSpam);

    // Check if a message uuid was provided in the location hash.
    var hashParams = new URLSearchParams(window.location.hash.slice(1));
    if (hashParams.has("uuid")) {
        // If it was, we should load that message.
        var uuid = hashParams.get("uuid");
        loadMessage(uuid);
    }

    // Register for when the hash location has changed.
    $(window).bind('hashchange', function(e) {
        // Check if the UUID is provided in the updated hash location.
        var hashParams = new URLSearchParams(window.location.hash.slice(1));
        if (hashParams.has("uuid")) {
            // If it was, we can load the message.
            var uuid = hashParams.get("uuid");
            loadMessage(uuid);
        }
    });
});
