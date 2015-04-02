
Smart Protocol definition
=========================

The smart protocol is a wire protocol that assumes a connection has been established between the client and a smart server application, and that there are reader and writer streams to go between them. Authentication and connection are handled elsewhere.

The exchange in principle
-------------------------

The protocol supports a series of exchanges so that the same communication pipe can be used for multiple operations. 

All exchanges of human-readable information, including base requests and responses, are in [JSON-RPC 2.0 format](http://www.jsonrpc.org/specification).

However when binary content needs to be exchanged, rather than embed the data in a JSON-RPC structure which would require costly conversion to/from base64 or similar, raw binary content will be sent 'in between' JSON-RPC messages, following on from requests or acknowledgements. This is not strictly standard but it works much better both in terms of processing overhead and use of bandwidth. So a server response for a proposed upload from the client would be a kind of 'go ahead' signal, after which the client should transfer the number of bytes advertised in the request in a raw stream. The server will then respond with another confirmation once all the bytes are received. 

This is possible because:
1. We assume each connection has dedicated & persistent in/out streams
2. We assume the server component can handle raw data transfers (we are not limited by architectural constraints / higher level protocol wrappers)

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
|**Method**     | __download_file__|
|**Purpose**    | Download a single file (metadata or chunk). This does not deal with binary deltas, only with the simple chunked upload of big files. However the server is free to store these however it likes.|
|**Params**     | lobSHA (string) - the SHA of the binary file in question|
|               | type (string) - "meta" or "chunk"|
|               | chunk_idx (Number) - only applicable to chunks, the chunk number (16MB)|
|               | size (Number) - size in bytes|
|**Result**     | "OK" if server has the data to send.|
|**POST**       | Immediately after Result:"OK", a BINARY STREAM of bytes will be sent by the server to the client of length 'size' above. The client must read all the bytes.|
|**POST Result**| No post result is required to be sent from client to server on receipt of all the bytes (server doesn't care). After all bytes have been read the server is ready for a new command.|

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
|**Method**     | __generate_lob_delta__|
|**Purpose**    | Ask the server to generate a binary patch between 2 lobs which we know it has (or re-use an existing delta).|
|**Params**     | baseLobSHA (string) - the SHA of the binary file content to use as a base|
|               | targetLobSHA (string) - the SHA of the binary file content we want to reconstruct from base + delta|
|**Result**     | size (Number) - size in bytes of delta if server has generated it ready to to send. Server should keep this calculated delta for a while, at least 1 day (maybe longer to re-use for multiple clients). 0 if there was a problem (error identifies). The client should subsequently request the calculated delta if it wants it (may choose not to if borderline)|

|||
|-----------|-------------|
|**Method**     | __download_lob_delta__|
|**Purpose**    | Ask to download a binary patch between 2 lobs which has already been calculated with __generate_lob_delta__. This method is separate from __generate_lob_delta__ so client explicitly chooses whether to use the delta or not once it knows how big it is; whether we bother depends on size & client transfer speed (deltas are not resumable)|
|**Params**     | baseLobSHA (string) - the SHA of the binary file content to use as a base|
|               | targetLobSHA (string) - the SHA of the binary file content we want to reconstruct from base + delta|
|**Result**     | size (Number) - size in bytes of the delta that's going to be sent|  
|               | metadata (embedded JSON metadata struct) - the content of the _meta file embedded in result which the client can save, to go with the content which will be coming next.|
|**POST**       | Immediately after a non-error result, a BINARY STREAM of bytes will be sent by the server to the client of length 'size' as indicated above. The client must read all the bytes.|
|**POST Result**| No post result is required to be sent from client to server on receipt of all the bytes (server doesn't care). After all bytes have been read the server is ready for a new command. The client must generate the revised target LOB from the delta, check the SHA is correct then split it into chunks|

