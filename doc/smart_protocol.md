
Smart Protocol definition
=========================

The smart protocol is a system by which the git-lob client and server exchange data, including potentially binary deltas, in order to fulfil what the 'dumb' sync protocol does just using file operations, more efficiently.

A reference implementation is provided for the server end of this exchange, [git-lob-serve](git-lob-serve.md)

The exchange in principle
-------------------------

The protocol supports a series of request/response pairs. The protocol does not assume whether or not those requests are issued over a single connection (e.g. a persistent ssh connection to a server side tool), or whether each one is issued as a separate request/response (or even broken into multiple requests/responses internally). Similarly the transport format is not predefined. Each implementor of the smart protocol can use a low-level transport format of its choosing. 

As such the providers.smart.Transport class abstracts to the level of individual requests, and each implementation of Transport can fulfil these as it sees fit. This generally boils down to 2 broad categories: persistent transports and transient transports.

Persistent transport
--------------------
Persistent transports establish a connection once (barring any errors or timeouts) and perform a series of operations over that same connection, avoiding the need to incur the overhead of negotiating afresh on each request. The most common example of this is using an SSH connection with a server-side executable and using it for multiple exchanges of data. 

We provide a single PersistentTransport implementation which has pluggable I/O streams; initially just SSH but any connection which provides an io.ReadWriteCloser can serve as a connection.

The persistent transport sends readable / descriptive data in JSON format. JSON-RPC was an option but it's not required since we break some rules and it's a dedicated connection.

However when binary content needs to be exchanged, rather than embed the data in a JSON structure which would require costly conversion to/from base64 or similar, raw binary content will be downloaded as a response to a JSON request, or uploaded directly following a confirmation response from the server. This is not strictly standard but it works much better both in terms of processing overhead and use of bandwidth. So a server response for a proposed upload from the client would be a kind of 'go ahead' signal, after which the client should transfer the number of bytes advertised in the request in a raw stream. The server will then respond with another confirmation once all the bytes are received. 

Wrapping in JSON or even in lower level wrappers like protocol buffers would make binary streaming much less efficient, since these systems tend to require all the data to be present to decode a record. By streaming the binary directly we free the server and client from that, so they can stream binary content to files directly if they want.

All JSON request and response structures must be terminated with a binary 0 in the stream to indicate termination of the JSON, this allows efficient reading of variable-length data within a persistent re-usable stream.

Transient transport
-------------------
Transient transports don't maintain a connection between requests, meaning each one goes through the full stack. This is a requirement for REST and similar back-ends (not yet implemented). In this case, the protocol will be wrapped as appropriate for that transport (e.g. REST may translate the method to an endpoint and request arguments to URL params).
Because each transient transport will package this differently, the protocol is not predefined as is it with the persistent transports.

File exchanges
--------------

The smart protocol still supports the same simple chunked file upload/download of the basic sync; originally this was used because it allows some level of resumption of data transfers while requiring no server-side logic. However it's also a convenient & simple approach even when you do have server side logic, for cases where either binary deltas are not supported, or they don't make sense (too much divergence or no base file). It means we don't have to build a custom download resume feature just for the smart protocol.

However, smart server implementations are free to store the data however it likes instead of mirroring the client file structure. Instead of sending chunks by file name, the data is sent with information about what type it is and what chunk number it is, and the server is free to store that however it likes, so long as it can retrieve it on that basis again later.

Protocol methods
----------------
|||
|-----------|-------------|
| **Method** | __QueryCaps__ |
| **Purpose**| Asks the server to return its supported capabilities|
| **Params** | None|
| **Result** | Array of strings identifying capabilities the server supports. So far only one is defined: "binary_delta"|

|||
|-----------|-------------|
|**Method** | __SetEnabledCaps__ |
|**Purpose**| Tells the server that the client wants to enable a list of capabilities. All omitted caps are assumed to be disabled|
|**Params**|  EnableCaps: Array of strings identifying caps to enable, must have been present in query_caps response.|
|**Result**|  Error is empty on success (error should also be populated on error)|

|||
|-----------|-------------|
|**Method**  | __FileExists__ |
|**Purpose** |Find out whether a given file (metadata or chunk) exists on the server already|
|**Params**  |LobSHA (string): the SHA of the binary file in question|
|            |Type (string): "meta" or "chunk"|
|            |ChunkIdx (Number): only applicable to chunks, the chunk number (16MB)|
|**Result**  |Exists: True or False|
|            |Size: Size of the file|

|||
|-----------|-------------|
|**Method**  | __LOBExists__ |
|**Purpose** |Find out whether a given LOB exists in its entirety (meta and all chunks of right size|
|**Params**  |LobSHA (string): the SHA of the binary file in question|
|**Result**  |Exists: True or False|
|            |Size: Size of the LOB content (excluding meta)|

|||
|-----------|-------------|
|**Method**  | __FileExistsOfSize__ |
|**Purpose** |Find out whether a given file (metadata or chunk) exists on the server already and is of the size specified|
|**Params**  |LobSHA (string): the SHA of the binary file in question|
|            |Type (string): "meta" or "chunk"|
|            |ChunkIdx (Number): only applicable to chunks, the chunk number (16MB)|
|            |Size (Number): size in bytes|
|**Result**  |Result: True or False|

|||
|-----------|-------------|
| **Method**      |__UploadFile__|
| **Purpose**     |Upload a single file (metadata or chunk). This does not deal with binary deltas, only with the simple chunked upload of big files. However the server is free to store these however it likes.|
| **Params**      |LobSHA (string): the SHA of the binary file in question|
|                 |Type (string): "meta" or "chunk"|
|                 |ChunkIdx (Number): only applicable to chunks, the chunk number (16MB)|
|                 |Size (Number): size in bytes|
| **Result**      |OKToSend: True if clear to send. Note server must accept upload if client requests it even if it has the file already (--force). Client will use file_exists_of_size to make it's own decision on whether to upload or not.|
| **POST**        |Immediately after OKToSend:True, a BINARY STREAM of bytes will be sent by the client to the server of length 'size' above.|
| **POST Result** |ReceivedOK: True if server received all the bytes and stored the file successfully. On failure, return Error.|

|||
|-----------|-------------|
|**Method**     | __DownloadFilePrepare__|
|**Purpose**    | Prepare to download a single file (metadata or chunk). This does not deal with binary deltas, only with the simple chunked download of big files. However the server is free to store these however it likes.|
|**Params**     | LobSHA (string): the SHA of the binary file in question|
|               | Type (string): "meta" or "chunk"|
|               | ChunkIdx (Number): only applicable to chunks, the chunk number (16MB)|
|**Result**     | Size: Byte size if server has the data to send (Error otherwise).|
|               | Client should follow up with a call to __DownloadFileStart__ to trigger the binary data send, which includes all the same params|

|||
|-----------|-------------|
|**Method**     | __DownloadFileStart__|
|**Purpose**    | Begin downloading a single file (metadata or chunk). This does not deal with binary deltas, only with the simple chunked download of big files. However the server is free to store these however it likes.|
|**Params**     | LobSHA (string): the SHA of the binary file in question|
|               | Type (string): "meta" or "chunk"|
|               | ChunkIdx (Number): only applicable to chunks, the chunk number (16MB)|
|               | Size (Number): size in bytes, as obtained from __DownloadFilePrepare__ which *must* be called first|
|**Result**     | A pure binary stream of data of exactly Size bytes. Client must read all the bytes.|


|||
|-----------|-------------|
|**Method**  |__PickCompleteLOB__|
|**Purpose** |Out of a list of LOB SHAs in order of preference, return which one (if any) the server has a complete copy of already. This is used to probe for previous versions of a file to exchange a binary delta of. Note that in all cases (upload and download) the client is responsible for creating the list of possible ancestor candidates, whether sending or receiving. This means the server doesn't have to have the git repo available, and the client always has the git commits when downloading anyway (that's how it decides what to download)|
|**Params**  |LobSHAs: array of strings identifying LOBs in order of preference (usually ancestors of a file)|
|**Result**  |FirstSHA: first sha in the list that server has a complete file copy of, or blank string if none are present. The server should confirm that all data is present but does not need to check the sha integrity (done post delta application)|

|||
|-----------|-------------|
|**Method**     | __UploadDelta__|
|**Purpose**    | Ask to upload a binary patch between 2 lobs which the client has calculated so the server can apply it to its own store, without uploading the entire file content. This is only about the chunk content; metadata is uploaded with __UploadFile__ as usual and should be done before calling this method.|
|**Params**     | BaseLobSHA (string): the SHA of the binary file content to use as a base. Client should have already identified that server has this via __PickCompleteLOB__|
|               | TargetLobSHA (string): the SHA of the binary file content we want to reconstruct from base + delta|
|               | Size (Number): size in bytes of the binary delta|
|**Result**     | OKToSend: True if server is ready to receive delta on this basis|
|**POST**       | Immediately after Result:True, a BINARY STREAM of bytes will be sent by the client to the server of length 'size' above. The server must read all the bytes and then generate the final file from the delta + base (must check SHA integrity) and store it.|
| **POST Result** |ReceivedOK: True if server received all the bytes and stored the file successfully. On failure, return Error.|

|||
|-----------|-------------|
|**Method**     | __DownloadDeltaPrepare__|
|**Purpose**    | Ask the server to generate a binary patch between 2 lobs which we know it has (or re-use an existing delta). This is only about the chunk content; metadata is downloaded the usual way.|
|**Params**     | BaseLobSHA (string): the SHA of the binary file content to use as a base|
|               | TargetLobSHA (string): the SHA of the binary file content we want to reconstruct from base + delta|
|**Result**     | Size (Number): size in bytes of delta if server has generated it ready to to send (Error otherwise). Server should keep this calculated delta for a while, at least 1 day (maybe longer to re-use for multiple clients). 0 if there was a problem (error identifies). The client should subsequently request the calculated delta if it wants it (may choose not to if borderline)|
|               | Client should follow up with a call to __DownloadDeltaStart__ to trigger the binary data send, which includes all the same params|

|||
|-----------|-------------|
|**Method**     | __DownloadDeltaStart__|
|**Purpose**    | Begin downloading a LOB delta file to apply locally against a base LOB to generate new content. Metadata is not included, that's downloaded the usual way|
|**Params**     | BaseLobSHA (string): the SHA of the binary file content to use as a base|
|               | TargetLobSHA (string): the SHA of the binary file content we want to reconstruct from base + delta|
|               | Size (Number): size in bytes of delta as reported from __DownloadDeltaPrepare__.| 
|**Result**     | A pure binary stream of data of exactly Size bytes. Client must read all the bytes and use to apply to base LOB to create new content.|

|||
|-----------|-------------|
|**Method**     | __Exit__|
|**Purpose**    | Exit the server-side process|
|**Params**     |None|
|**Result**     |None|
