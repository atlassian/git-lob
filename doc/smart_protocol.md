
Smart Protocol definition
=========================

The smart protocol is a system by which the git-lob client and server exchange data, including potentially binary deltas, in order to fulfil what the 'dumb' sync protocol does just using file operations, more efficiently.

The exchange in principle
-------------------------

The protocol supports a series of request/response pairs. The protocol does not assume whether or not those requests are issued over a single connection (e.g. a persistent ssh connection to a server side tool), or whether each one is issued as a separate request/response (or even broken into multiple requests/responses internally). Similarly the transport format is not predefined. Each implementor of the smart protocol can use a low-level transport format of its choosing. 

As such the providers.smart.Transport class abstracts to the level of individual requests, and each implementation of Transport can fulfil these as it sees fit. This generally boils down to 2 broad categories: persistent transports and transient transports.

Persistent transport
--------------------
Persistent transports establish a connection once (barring any errors or timeouts) and perform a series of operations over that same connection, avoiding the need to incur the overhead of negotiating afresh on each request. The most common example of this is using an SSH connection with a server-side executable and using it for multiple exchanges of data. 

We provide a single PersistentTransport implementation which has pluggable I/O streams; initially just SSH but any connection which provides an io.ReadWriteCloser can serve as a connection.

The persistent transport sends descriptive data in [JSON-RPC 2.0 format](http://www.jsonrpc.org/specification).

However when binary content needs to be exchanged, rather than embed the data in a JSON-RPC structure which would require costly conversion to/from base64 or similar, raw binary content will be downloaded as a response to a JSON-RPC request, or uploaded directly following a confirmation response from the server. This is not strictly standard but it works much better both in terms of processing overhead and use of bandwidth. So a server response for a proposed upload from the client would be a kind of 'go ahead' signal, after which the client should transfer the number of bytes advertised in the request in a raw stream. The server will then respond with another confirmation once all the bytes are received. 

Wrapping in JSON or even in lower level wrappers like protocol buffers would make binary streaming much less efficient, since these systems tend to require all the data to be present to decode a record. By streaming the binary directly we free the server and client from that, so they can stream binary content to files directly if they want.

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
| **Method** | __query_caps__ |
| **Purpose**| Asks the server to return its supported capabilities|
| **Params** | None|
| **Result** | Array of strings identifying capabilities the server supports. So far only one is defined: "binary_delta"|

|||
|-----------|-------------|
|**Method** | __set_caps__|
|**Purpose**| Tells the server that the client wants to enable a list of capabilities. All omitted caps are assumed to be disabled|
|**Params**|  Array of strings identifying caps to enable, must have been present in query_caps response.|
|**Result**|  "OK" on success (error should also be populated on error)|

|||
|-----------|-------------|
|**Method**  |__file_exists__|
|**Purpose** |Find out whether a given file (metadata or chunk) exists on the server already|
|**Params**  |lobSHA (string) - the SHA of the binary file in question|
|            |type (string) - "meta" or "chunk"|
|            |chunk_idx (Number) - only applicable to chunks, the chunk number (16MB)|
|**Result**  |True or False|

|||
|-----------|-------------|
|**Method**  |__file_exists_of_size__|
|**Purpose** |Find out whether a given file (metadata or chunk) exists on the server already and is of the size specified|
|**Params**  |lobSHA (string) - the SHA of the binary file in question|
|            |type (string) - "meta" or "chunk"|
|            |chunk_idx (Number) - only applicable to chunks, the chunk number (16MB)|
|            |size (Number) - size in bytes|
|**Result**  |True or False|

|||
|-----------|-------------|
| **Method**      |__upload_file__|
| **Purpose**     |Upload a single file (metadata or chunk). This does not deal with binary deltas, only with the simple chunked upload of big files. However the server is free to store these however it likes.|
| **Params**      |lobSHA (string) - the SHA of the binary file in question|
|                 |type (string) - "meta" or "chunk"|
|                 |chunk_idx (Number) - only applicable to chunks, the chunk number (16MB)|
|                 |size (Number) - size in bytes|
| **Result**      |OK if clear to send. Note server must accept upload if client requests it even if it has the file already (--force). Client will use file_exists_of_size to make it's own decision on whether to upload or not.|
| **POST**        |Immediately after Result:"OK", a BINARY STREAM of bytes will be sent by the client to the server of length 'size' above.|
| **POST Result** |"OK" if server received all the bytes and stored the file successfully|

|||
|-----------|-------------|
|**Method**     | __download_file_prepare__|
|**Purpose**    | Prepare to download a single file (metadata or chunk). This does not deal with binary deltas, only with the simple chunked download of big files. However the server is free to store these however it likes.|
|**Params**     | lobSHA (string) - the SHA of the binary file in question|
|               | type (string) - "meta" or "chunk"|
|               | chunk_idx (Number) - only applicable to chunks, the chunk number (16MB)|
|**Result**     | "OK" and byte size if server has the data to send. Client should follow up with __download_file_confirm__|

|||
|-----------|-------------|
|**Method**     | __download_file_confirm__|
|**Purpose**    | Download a single file (metadata or chunk). This does not deal with binary deltas, only with the simple chunked download of big files. However the server is free to store these however it likes.|
|**POST**       | A BINARY STREAM of bytes will be sent by the server to the client of length 'size' as reported from __download_file_prepare__. The client must read all the bytes.|

|||
|-----------|-------------|
|**Method**  |__pick_complete_lob__|
|**Purpose** |Out of a list of LOB SHAs in order of preference, return which one (if any) the server has a complete copy of already. This is used to probe for previous versions of a file to exchange a binary delta of. Note that in all cases (upload and download) the client is responsible for creating the list of possible ancestor candidates, whether sending or receiving. This means the server doesn't have to have the git repo available, and the client always has the git commits when downloading anyway (that's how it decides what to download)|
|**Params**  |lobshas - array of strings identifying LOBs in order of preference (usually ancestors of a file)|
|**Result**  |sha - first sha in the list that server has a complete file copy of. The server should confirm that all data is present but does not need to check the sha integrity (done post delta application)|

|||
|-----------|-------------|
|**Method**     | __upload_lob_delta__|
|**Purpose**    | Ask to upload a binary patch between 2 lobs which the client has calculated so the server can apply it to its own store, without uploading the entire file content.|
|**Params**     | baseLobSHA (string) - the SHA of the binary file content to use as a base. Client should have already identified that server has this via __pick_complete_lob__|
|               | targetLobSHA (string) - the SHA of the binary file content we want to reconstruct from base + delta|
|               | size (Number) - size in bytes of the binary delta|
|               | metadata (embedded JSON metadata struct) - the content of the _meta file to go with targetLobSHA|
|**Result**     | "OK" if server is ready to receive delta on this basis|
|**POST**       | Immediately after Result:"OK", a BINARY STREAM of bytes will be sent by the client to the server of length 'size' above. The server must read all the bytes and then generate the final file from the delta + base (must check SHA integrity) and store it.|
| **POST Result** |"OK" if server received all the delta bytes, generated the final file and stored it successfully|

|||
|-----------|-------------|
|**Method**     | __download_lob_delta_prepare__|
|**Purpose**    | Ask the server to generate a binary patch between 2 lobs which we know it has (or re-use an existing delta).|
|**Params**     | baseLobSHA (string) - the SHA of the binary file content to use as a base|
|               | targetLobSHA (string) - the SHA of the binary file content we want to reconstruct from base + delta|
|**Result**     | size (Number) - size in bytes of delta if server has generated it ready to to send. Server should keep this calculated delta for a while, at least 1 day (maybe longer to re-use for multiple clients). 0 if there was a problem (error identifies). The client should subsequently request the calculated delta if it wants it (may choose not to if borderline)|
|               | metadata (embedded JSON metadata struct) - the content of the meta file embedded in result which the client can save, to go with the content which will be downloaded from __download_lob_delta_confirm__|

|||
|-----------|-------------|
|**Method**     | __download_lob_delta_confirm__|
|**Purpose**    | Ask to download a binary patch between 2 lobs which has already been calculated with __download_lob_delta_prepare__. This method is separate so client explicitly chooses whether to use the delta or not once it knows how big it is; whether we bother depends on size & client transfer speed (deltas are not resumable). Also because we need to keep JSON and binary data separate|
|**Params**     | baseLobSHA (string) - the SHA of the binary file content to use as a base|
|               | targetLobSHA (string) - the SHA of the binary file content we want to reconstruct from base + delta|
|**POST**       | A BINARY STREAM of bytes will be sent by the server to the client of length 'size' as indicated by __download_lob_delta_prepare__. The client must read all the bytes.|

