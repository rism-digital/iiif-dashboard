module ProjectSearch exposing (matches)

import Domain exposing (Project)


matches : String -> Project -> Bool
matches query project =
    let
        keywords =
            query
                |> String.toLower
                |> String.words

        searchableText =
            [ project.name
            , project.id
            , project.homepage
            , Maybe.withDefault "" project.manifestUrl
            , Maybe.withDefault "" project.imageInfoUrl
            , Maybe.withDefault "" project.notes
            ]
                |> String.join " "
                |> String.toLower
    in
    List.all (\keyword -> String.contains keyword searchableText) keywords
