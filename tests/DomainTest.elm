module DomainTest exposing (tests)

import Dict
import Domain exposing (Status(..))
import Expect
import Json.Decode as Decode
import Test exposing (Test, describe, test)


tests : Test
tests =
    describe "dashboard contracts"
        [ test "decodes the contributor registry" <|
            \_ ->
                Decode.decodeString Domain.registryDecoder registryJson
                    |> Result.map (.projects >> List.map .id)
                    |> Expect.equal (Ok [ "example-library" ])
        , test "decodes generated observations" <|
            \_ ->
                Decode.decodeString Domain.resultsDecoder resultsJson
                    |> Result.map
                        (\results ->
                            results.projects
                                |> List.filter (\project -> project.id == "example-library")
                                |> List.head
                                |> Maybe.andThen (\project -> Dict.get "presentation.default" project.checks)
                                |> Maybe.map .status
                        )
                    |> Expect.equal (Ok (Just Pass))
        , test "decodes advisory observations from schema version 2" <|
            \_ ->
                Decode.decodeString Domain.resultsDecoder advisoryResultsJson
                    |> Result.map
                        (.projects
                            >> List.head
                            >> Maybe.andThen (\project -> Dict.get "presentation.compression" project.checks)
                            >> Maybe.map .status
                        )
                    |> Expect.equal (Ok (Just Advisory))
        , test "ranks advisories between warnings and passes" <|
            \_ ->
                [ Unknown, Pass, Advisory, Warning, Fail ]
                    |> List.map Domain.statusRank
                    |> Expect.equal [ 0, 1, 2, 3, 4 ]
        , test "labels advisory observations" <|
            \_ ->
                Domain.statusLabel Advisory
                    |> Expect.equal "Advisory"
        , test "decodes CORS headers as raw strings" <|
            \_ ->
                Decode.decodeString Domain.resultsDecoder resultsJson
                    |> Result.map
                        (\results ->
                            results.projects
                                |> List.head
                                |> Maybe.andThen (\project -> Dict.get "presentation.default" project.checks)
                                |> Maybe.map .corsHeaders
                        )
                    |> Expect.equal (Ok (Just [ "access-control-allow-origin: *", "access-control-allow-origin: *" ]))
        , test "decodes full default response headers" <|
            \_ ->
                Decode.decodeString Domain.resultsDecoder resultsJson
                    |> Result.map
                        (\results ->
                            results.projects
                                |> List.head
                                |> Maybe.andThen (\project -> Dict.get "presentation.default" project.checks)
                                |> Maybe.map .responseHeaders
                        )
                    |> Expect.equal (Ok (Just [ "Content-Type: application/ld+json", "Vary: Accept" ]))
        , test "decodes redirect response headers" <|
            \_ ->
                Decode.decodeString Domain.resultsDecoder resultsJson
                    |> Result.map
                        (.projects
                            >> List.head
                            >> Maybe.andThen (\project -> Dict.get "presentation.default" project.checks)
                            >> Maybe.map
                                (.redirectChain
                                    >> List.head
                                    >> Maybe.map (\hop -> ( hop.url, hop.httpStatus, ( hop.location, hop.responseHeaders ) ))
                                )
                        )
                    |> Expect.equal
                        (Ok
                            (Just
                                (Just
                                    ( "https://example.org/start"
                                    , 302
                                    , ( Just "/manifest", [ "Location: /manifest", "Set-Cookie: redirect=1" ] )
                                    )
                                )
                            )
                        )
        , test "defaults response headers in older snapshots to an empty list" <|
            \_ ->
                Decode.decodeString Domain.resultsDecoder oldResultsJson
                    |> Result.map
                        (.projects
                            >> List.head
                            >> Maybe.andThen (\project -> Dict.get "presentation.default" project.checks)
                            >> Maybe.map .responseHeaders
                        )
                    |> Expect.equal (Ok (Just []))
        , test "defaults redirect chains in older snapshots to an empty list" <|
            \_ ->
                Decode.decodeString Domain.resultsDecoder oldResultsJson
                    |> Result.map
                        (.projects
                            >> List.head
                            >> Maybe.andThen (\project -> Dict.get "presentation.default" project.checks)
                            >> Maybe.map .redirectChain
                        )
                    |> Expect.equal (Ok (Just []))
        , test "defaults older project results to checked" <|
            \_ ->
                Decode.decodeString Domain.resultsDecoder resultsJson
                    |> Result.map (.projects >> List.head >> Maybe.map (\project -> ( project.checked, project.checkedAt )))
                    |> Expect.equal (Ok (Just ( True, Just "2026-01-01T00:00:00Z" )))
        , test "decodes an explicit unchecked project result" <|
            \_ ->
                Decode.decodeString Domain.resultsDecoder uncheckedResultsJson
                    |> Result.map (.projects >> List.head >> Maybe.map (\project -> ( project.checked, project.checkedAt, Dict.isEmpty project.checks )))
                    |> Expect.equal (Ok (Just ( False, Nothing, True )))
        , test "decodes a project without an image sample" <|
            \_ ->
                Decode.decodeString Domain.registryDecoder manifestOnlyRegistryJson
                    |> Result.map (.projects >> List.head >> Maybe.andThen .imageInfoUrl)
                    |> Expect.equal (Ok Nothing)
        , test "decodes a project with only an image sample" <|
            \_ ->
                Decode.decodeString Domain.registryDecoder imageOnlyRegistryJson
                    |> Result.map
                        (.projects
                            >> List.head
                            >> Maybe.map (\project -> ( project.manifestUrl, project.imageInfoUrl ))
                        )
                    |> Expect.equal (Ok (Just ( Nothing, Just "https://images.example/info.json" )))
        ]


registryJson : String
registryJson =
    "{\"schemaVersion\":1,\"projects\":[{\"id\":\"example-library\",\"name\":\"Example Library\",\"homepage\":\"https://example.org\",\"manifestUrl\":\"https://example.org/manifest\",\"imageInfoUrl\":\"https://example.org/info.json\"}]}"


resultsJson : String
resultsJson =
    "{\"schemaVersion\":1,\"generatedAt\":\"2026-01-01T00:00:00Z\",\"projects\":[{\"id\":\"example-library\",\"name\":\"Example Library\",\"checkedAt\":\"2026-01-01T00:00:00Z\",\"checks\":{\"presentation.default\":{\"status\":\"pass\",\"summary\":\"Valid\",\"httpStatus\":200,\"detected\":\"v3\",\"contentType\":\"application/ld+json\",\"corsHeaders\":[\"access-control-allow-origin: *\",\"access-control-allow-origin: *\"],\"responseHeaders\":[\"Content-Type: application/ld+json\",\"Vary: Accept\"],\"redirectChain\":[{\"url\":\"https://example.org/start\",\"httpStatus\":302,\"location\":\"/manifest\",\"responseHeaders\":[\"Location: /manifest\",\"Set-Cookie: redirect=1\"]}]}}}]}"


oldResultsJson : String
oldResultsJson =
    "{\"schemaVersion\":1,\"generatedAt\":\"2026-01-01T00:00:00Z\",\"projects\":[{\"id\":\"example-library\",\"name\":\"Example Library\",\"checks\":{\"presentation.default\":{\"status\":\"pass\",\"summary\":\"Valid\",\"corsHeaders\":[]}}}]}"


advisoryResultsJson : String
advisoryResultsJson =
    "{\"schemaVersion\":2,\"generatedAt\":\"2026-01-01T00:00:00Z\",\"projects\":[{\"id\":\"example-library\",\"name\":\"Example Library\",\"checks\":{\"presentation.compression\":{\"status\":\"advisory\",\"summary\":\"Use gzip\",\"corsHeaders\":[]}}}]}"


manifestOnlyRegistryJson : String
manifestOnlyRegistryJson =
    "{\"schemaVersion\":1,\"projects\":[{\"id\":\"manifest-only\",\"name\":\"manifest.example\",\"homepage\":\"https://manifest.example/\",\"manifestUrl\":\"https://manifest.example/manifest\"}]}"


imageOnlyRegistryJson : String
imageOnlyRegistryJson =
    "{\"schemaVersion\":1,\"projects\":[{\"id\":\"image-only\",\"name\":\"images.example\",\"homepage\":\"https://images.example/\",\"imageInfoUrl\":\"https://images.example/info.json\"}]}"


uncheckedResultsJson : String
uncheckedResultsJson =
    "{\"schemaVersion\":1,\"generatedAt\":\"2026-01-01T00:00:00Z\",\"projects\":[{\"id\":\"example-library\",\"name\":\"Example Library\",\"checked\":false,\"checks\":{}}]}"
