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
                , "image.response"
                , "image.response-preflight"
                ]
                    |> List.filter (\key -> Main.ruleReferences key (check Pass (Just 200)) |> List.isEmpty)
                    |> Expect.equal []
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
