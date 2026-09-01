module MainTest exposing (tests)

import Dict
import Domain exposing (CheckResult, Project, Status(..))
import Expect
import Main exposing (VersionState(..))
import Test exposing (Test, describe, test)


tests : Test
tests =
    describe "dashboard status presentation"
        [ test "renders a successful 406 negotiation as unavailable" <|
            \_ ->
                Dict.singleton "presentation.v3" (check Pass (Just 406))
                    |> Main.negotiatedVersionState "presentation" "v3"
                    |> Expect.equal (VersionUnavailable "HTTP 406")
        , test "renders advisory negotiation results as unavailable" <|
            \_ ->
                Dict.singleton "presentation.v3" (check Advisory (Just 200))
                    |> Main.negotiatedVersionState "presentation" "v3"
                    |> Expect.equal (VersionUnavailable "requested representation not returned")
        , test "matching negotiated versions remain green when the response has a warning" <|
            \_ ->
                let
                    result =
                        check Warning (Just 200)
                in
                Dict.singleton "image.v3" { result | detected = Just "v3 Level 2" }
                    |> Main.negotiatedVersionState "image" "v3"
                    |> Expect.equal (VersionResult Pass)
        , test "a warning response containing another version remains unavailable" <|
            \_ ->
                let
                    result =
                        check Warning (Just 200)
                in
                Dict.singleton "image.v3" { result | detected = Just "v2 Level 2" }
                    |> Main.negotiatedVersionState "image" "v3"
                    |> Expect.equal (VersionUnavailable "received v2 Level 2")
        , test "advisories do not downgrade the core aggregate" <|
            \_ ->
                Dict.fromList
                    [ ( "image.default", check Pass (Just 200) )
                    , ( "image.compression", check Advisory (Just 200) )
                    , ( "image.v3", check Fail (Just 500) )
                    ]
                    |> Main.coreStatus "image"
                    |> Expect.equal Pass
        , test "warnings outrank advisories in the core aggregate" <|
            \_ ->
                Dict.fromList
                    [ ( "presentation.default", check Warning (Just 200) )
                    , ( "presentation.compression", check Advisory (Just 200) )
                    ]
                    |> Main.coreStatus "presentation"
                    |> Expect.equal Warning
        , test "health meter uses fixed primary-request slots and excludes preflight" <|
            \_ ->
                Dict.fromList
                    [ ( "presentation.default", check Pass (Just 200) )
                    , ( "presentation.compression", check Advisory (Just 200) )
                    , ( "presentation.v2", check Warning (Just 200) )
                    , ( "presentation.preflight", check Fail (Just 403) )
                    , ( "image.default", check Pass (Just 200) )
                    , ( "image.compression", check Pass (Just 200) )
                    , ( "image.v2", check Pass (Just 200) )
                    , ( "image.v3", check Pass (Just 200) )
                    , ( "image.base-redirect", check Fail (Just 500) )
                    , ( "image.base-slash-redirect", check Advisory (Just 404) )
                    , ( "image.info-preflight", check Fail (Just 403) )
                    ]
                    |> Main.healthStatuses bothApisProject
                    |> Expect.equal
                        [ Pass
                        , Advisory
                        , Warning
                        , Unknown
                        , Pass
                        , Pass
                        , Pass
                        , Pass
                        , Fail
                        , Advisory
                        , Unknown
                        ]
        , test "the base URI redirect cites the exact IIIF recommendation" <|
            \_ ->
                Main.ruleReferences "image.base-redirect" (check Pass (Just 303))
                    |> Expect.equal
                        [ ( "How this check works", "./checks.html#image-base-redirect" )
                        , ( "Image API 3.0 §2", "https://iiif.io/api/image/3.0/#2-uri-syntax" )
                        ]
        , test "every displayed server-behavior rule has a reference" <|
            \_ ->
                [ "presentation.default"
                , "presentation.compression"
                , "presentation.v2"
                , "presentation.v3"
                , "presentation.preflight"
                , "image.default"
                , "image.compression"
                , "image.v2"
                , "image.v3"
                , "image.info-preflight"
                , "image.base-redirect"
                , "image.base-slash-redirect"
                , "image.response"
                , "image.response-preflight"
                ]
                    |> List.filter (\key -> Main.ruleReferences key (check Pass (Just 200)) |> List.isEmpty)
                    |> Expect.equal []
        , test "viewer links safely encode the complete Manifest URL" <|
            \_ ->
                Main.viewerUrl "https://example.org/manifest?version=3&lang=en"
                    |> Expect.equal "./viewer.html?manifest=https%3A%2F%2Fexample.org%2Fmanifest%3Fversion%3D3%26lang%3Den"
        , test "formats service test durations compactly" <|
            \_ ->
                [ Main.formatDuration 842
                , Main.formatDuration 1234
                , Main.formatDuration 75321
                ]
                    |> Expect.equal [ "842 ms", "1.2 s", "1m 15s" ]
        ]


bothApisProject : Project
bothApisProject =
    { id = "example"
    , name = "Example"
    , homepage = "https://example.org"
    , manifestUrl = Just "https://example.org/manifest"
    , imageInfoUrl = Just "https://example.org/info.json"
    , notes = Nothing
    , skip = False
    }


check : Status -> Maybe Int -> CheckResult
check status httpStatus =
    { status = status
    , summary = ""
    , httpStatus = httpStatus
    , detected = Nothing
    , contentType = Nothing
    , corsHeaders = []
    , location = Nothing
    , requestAccept = Nothing
    , responseHeaders = []
    , redirectChain = []
    }
